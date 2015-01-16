package sse_test

import (
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/vito/go-sse/sse"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("EventSource", func() {
	var (
		server *ghttp.Server

		source *EventSource
	)

	BeforeEach(func() {
		server = ghttp.NewServer()
		url := server.URL()

		source = &EventSource{
			Client:               http.DefaultClient,
			DefaultRetryInterval: 100 * time.Millisecond,
			CreateRequest: func() *http.Request {
				request, err := http.NewRequest("GET", url, nil)
				Ω(err).ShouldNot(HaveOccurred())

				return request
			},
		}
	})

	Context("when connecting explicitly", func() {
		Context("when the server returns ok", func() {
			BeforeEach(func() {
				server.RouteToHandler("GET", "/", ghttp.RespondWith(http.StatusOK, ""))
			})

			It("does not error", func() {
				err := source.Connect()
				Ω(err).ShouldNot(HaveOccurred())
			})
		})

		// See http://www.w3.org/TR/eventsource/#processing-model for
		// details on re-establishing the connection
		Context("when the server returns a non-retryable error", func() {
			BeforeEach(func() {
				server.RouteToHandler("GET", "/", ghttp.RespondWith(http.StatusBadRequest, ""))
			})

			It("returns a BadResponseError", func() {
				err := source.Connect()
				Ω(err).Should(BeAssignableToTypeOf(BadResponseError{}))
			})
		})

		Context("when the server is not running", func() {
			BeforeEach(func() {
				server.RouteToHandler("GET", "/", ghttp.RespondWith(http.StatusOK, ""))
				server.Close()
			})

			It("retries indefinitely", func() {
				doneChan := make(chan struct{})

				go func() {
					source.Connect()
					close(doneChan)
				}()

				Consistently(doneChan).ShouldNot(BeClosed())
			})
		})

		// See http://www.w3.org/TR/eventsource/#processing-model for
		// details on re-establishing the connection
		Context("when the server consistently returns retryable errors", func() {
			BeforeEach(func() {
				server.RouteToHandler("GET", "/", ghttp.RespondWith(http.StatusInternalServerError, ""))
			})

			It("retries indefinitely", func() {
				doneChan := make(chan struct{})

				go func() {
					source.Connect()
					close(doneChan)
				}()

				Consistently(doneChan).ShouldNot(BeClosed())
			})
		})
	})

	Context("when the connection breaks after events are read", func() {
		BeforeEach(func() {
			server.AppendHandlers(
				func(w http.ResponseWriter, r *http.Request) {
					flusher := w.(http.Flusher)
					closeNotify := w.(http.CloseNotifier).CloseNotify()

					w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
					w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Add("Connection", "keep-alive")

					w.WriteHeader(http.StatusOK)

					flusher.Flush()

					Event{
						ID:   "1",
						Data: []byte("hello"),
					}.Write(w)

					flusher.Flush()

					Event{
						ID:   "2",
						Data: []byte("hello again"),
					}.Write(w)

					flusher.Flush()

					<-closeNotify
				},
				ghttp.CombineHandlers(
					ghttp.VerifyHeaderKV("Last-Event-ID", "2"),
					func(w http.ResponseWriter, r *http.Request) {
						flusher := w.(http.Flusher)

						w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
						w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Add("Connection", "keep-alive")

						w.WriteHeader(http.StatusOK)

						flusher.Flush()

						Event{
							ID:   "3",
							Data: []byte("welcome back"),
						}.Write(w)

						flusher.Flush()
					},
				),
			)
		})

		It("reconnects from the last event id", func() {
			Ω(source.Next()).Should(Equal(Event{
				ID:   "1",
				Data: []byte("hello"),
			}))

			Ω(source.Next()).Should(Equal(Event{
				ID:   "2",
				Data: []byte("hello again"),
			}))

			server.CloseClientConnections()

			Ω(source.Next()).Should(Equal(Event{
				ID:   "3",
				Data: []byte("welcome back"),
			}))

			_, err := source.Next()
			Ω(err).Should(Equal(io.EOF))
		})
	})

	Context("when reconnecting continuously fails", func() {
		var retryTimes <-chan time.Time

		BeforeEach(func() {
			times := make(chan time.Time, 10)
			retryTimes = times

			server.AppendHandlers(
				func(w http.ResponseWriter, r *http.Request) {
					flusher := w.(http.Flusher)

					w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
					w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Add("Connection", "keep-alive")

					w.WriteHeader(http.StatusOK)

					flusher.Flush()

					Event{
						ID:   "1",
						Data: []byte("hello"),
					}.Write(w)

					flusher.Flush()

					server.CloseClientConnections()
				},
				func(w http.ResponseWriter, r *http.Request) {
					times <- time.Now()
					server.CloseClientConnections()
				},
				func(w http.ResponseWriter, r *http.Request) {
					times <- time.Now()
					server.CloseClientConnections()
				},
				func(w http.ResponseWriter, r *http.Request) {
					times <- time.Now()
					server.CloseClientConnections()
				},
				func(w http.ResponseWriter, r *http.Request) {
					times <- time.Now()

					flusher := w.(http.Flusher)

					w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
					w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Add("Connection", "keep-alive")

					w.WriteHeader(http.StatusOK)

					flusher.Flush()

					Event{
						ID:   "2",
						Data: []byte("welcome back"),
					}.Write(w)

					flusher.Flush()

					Event{
						ID:    "3",
						Data:  []byte("see you in a bit"),
						Retry: 200 * time.Millisecond,
					}.Write(w)

					flusher.Flush()

					server.CloseClientConnections()
				},
				func(w http.ResponseWriter, r *http.Request) {
					times <- time.Now()
					server.CloseClientConnections()
				},
				func(w http.ResponseWriter, r *http.Request) {
					times <- time.Now()

					flusher := w.(http.Flusher)

					w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
					w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
					w.Header().Add("Connection", "keep-alive")

					w.WriteHeader(http.StatusOK)

					flusher.Flush()

					Event{
						ID:   "4",
						Data: []byte("hello again"),
					}.Write(w)

					flusher.Flush()
				},
			)
		})

		It("retries on the base interval, followed by the server-specified interval", func() {
			var time1, time2, time3, time4, time5 time.Time

			Ω(source.Next()).Should(Equal(Event{
				ID:   "1",
				Data: []byte("hello"),
			}))

			Ω(source.Next()).Should(Equal(Event{
				ID:   "2",
				Data: []byte("welcome back"),
			}))

			Ω(retryTimes).Should(Receive(&time1))
			Ω(retryTimes).Should(Receive(&time2))
			Ω(retryTimes).Should(Receive(&time3))

			Ω(source.Next()).Should(Equal(Event{
				ID:    "3",
				Data:  []byte("see you in a bit"),
				Retry: 200 * time.Millisecond,
			}))

			Ω(retryTimes).Should(Receive(&time4))

			Ω(source.Next()).Should(Equal(Event{
				ID:   "4",
				Data: []byte("hello again"),
			}))

			Ω(retryTimes).Should(Receive(&time5))

			Ω(time5.Sub(time4)).Should(BeNumerically("~", 200*time.Millisecond, 50*time.Millisecond))
			Ω(time4.Sub(time3)).Should(BeNumerically("~", 100*time.Millisecond, 50*time.Millisecond))
			Ω(time3.Sub(time2)).Should(BeNumerically("~", 100*time.Millisecond, 50*time.Millisecond))
			Ω(time2.Sub(time1)).Should(BeNumerically("~", 100*time.Millisecond, 50*time.Millisecond))
		})
	})

	Context("when the server returns 404", func() {
		BeforeEach(func() {
			server.AppendHandlers(
				ghttp.RespondWith(404, ""),
			)
		})

		It("returns a BadResponseError", func() {
			_, err := source.Next()
			Ω(err).Should(BeAssignableToTypeOf(BadResponseError{}))
		})
	})

	for _, retryableStatus := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		status := retryableStatus

		Context(fmt.Sprintf("when the server returns %d", status), func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.RespondWith(status, ""),
					func(w http.ResponseWriter, r *http.Request) {
						flusher := w.(http.Flusher)

						w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
						w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Add("Connection", "keep-alive")

						w.WriteHeader(http.StatusOK)

						flusher.Flush()

						Event{
							ID:   "1",
							Data: []byte("you made it!"),
						}.Write(w)

						flusher.Flush()
					},
				)
			})

			It("transparently reconnects", func() {
				Ω(source.Next()).Should(Equal(Event{
					ID:   "1",
					Data: []byte("you made it!"),
				}))
			})
		})
	}

	Describe("handling errors while reading events", func() {
		var eventChan chan Event
		var errChan chan error

		BeforeEach(func() {
			eventChan = make(chan Event, 1)
			errChan = make(chan error, 1)
		})

		Context("when the event reader is closed", func() {
			BeforeEach(func() {
				flushedChan := make(chan struct{})

				server.AppendHandlers(
					func(w http.ResponseWriter, r *http.Request) {
						closeNotify := w.(http.CloseNotifier).CloseNotify()
						flusher := w.(http.Flusher)

						w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
						w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Add("Connection", "keep-alive")

						w.WriteHeader(http.StatusOK)

						flusher.Flush()
						close(flushedChan)
						<-closeNotify
					},
					func(w http.ResponseWriter, r *http.Request) {
						flusher := w.(http.Flusher)

						w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
						w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Add("Connection", "keep-alive")

						w.WriteHeader(http.StatusOK)

						flusher.Flush()

						Event{
							ID:   "2",
							Data: []byte("hello again"),
						}.Write(w)

						flusher.Flush()
					},
				)

				doneCh := make(chan struct{})
				go func() {
					event, err := source.Next()
					if err != nil {
						errChan <- err
					} else {
						eventChan <- event
					}
					close(doneCh)
				}()

				<-flushedChan
				err := source.Close()
				Ω(err).ShouldNot(HaveOccurred())
				<-doneCh
			})

			It("returns ErrReadFromClosedSource", func() {
				Eventually(errChan).Should(Receive(Equal(ErrReadFromClosedSource)))
			})

			Context("when trying to call Next again", func() {
				var err error

				BeforeEach(func() {
					_, err = source.Next()
				})

				It("immediately returns ErrReadFromClosedSource", func() {
					Ω(err).Should(Equal(ErrReadFromClosedSource))
				})
			})

			Context("when reconnecting", func() {
				BeforeEach(func() {
					err := source.Connect()
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("does not immediately return ErrReadFromClosedSource when reading from Next again", func() {
					event, err := source.Next()
					Ω(err).ShouldNot(HaveOccurred())
					Ω(event.ID).Should(Equal("2"))
				})
			})
		})

		Context("when there are no more events", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					func(w http.ResponseWriter, r *http.Request) {
						flusher := w.(http.Flusher)

						w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
						w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Add("Connection", "keep-alive")

						w.WriteHeader(http.StatusOK)

						flusher.Flush()
					},
					func(w http.ResponseWriter, r *http.Request) {
						flusher := w.(http.Flusher)
						closeNotify := w.(http.CloseNotifier).CloseNotify()

						w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
						w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Add("Connection", "keep-alive")

						w.WriteHeader(http.StatusOK)

						flusher.Flush()

						Event{
							ID:   "2",
							Data: []byte("hello again"),
						}.Write(w)

						flusher.Flush()

						<-closeNotify
					},
				)

				doneCh := make(chan struct{})
				go func() {
					event, err := source.Next()
					if err != nil {
						errChan <- err
					} else {
						eventChan <- event
					}
					close(doneCh)
				}()

				<-doneCh
			})

			It("returns io.EOF", func() {
				Eventually(errChan).Should(Receive(Equal(io.EOF)))
			})

			Context("when trying to call Next again", func() {
				var err error

				BeforeEach(func() {
					_, err = source.Next()
				})

				It("immediately returns ErrReadFromClosedSource", func() {
					Ω(err).Should(Equal(ErrReadFromClosedSource))
				})
			})

			Context("when reconnecting", func() {
				BeforeEach(func() {
					err := source.Connect()
					Ω(err).ShouldNot(HaveOccurred())
				})

				It("does not immediately error when reading from Next again", func() {
					event, err := source.Next()
					Ω(err).ShouldNot(HaveOccurred())
					Ω(event.ID).Should(Equal("2"))
				})
			})
		})

		// Context("when an unrecognized error occurs", func() {
		// 	BeforeEach(func() {
		// 		server.AppendHandlers(
		// 			func(w http.ResponseWriter, r *http.Request) { // Connect
		// 				flusher := w.(http.Flusher)
		// 				w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
		// 				w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
		// 				w.Header().Add("Connection", "keep-alive")

		// 				w.WriteHeader(http.StatusOK)

		// 				for {
		// 					w.Write([]byte("hello"))
		// 					flusher.Flush()
		// 				}
		// 			},
		// 			func(w http.ResponseWriter, r *http.Request) {
		// 				flusher := w.(http.Flusher)

		// 				w.Header().Add("Content-Type", "text/event-stream; charset=utf-8")
		// 				w.Header().Add("Cache-Control", "no-cache, no-store, must-revalidate")
		// 				w.Header().Add("Connection", "keep-alive")

		// 				w.WriteHeader(http.StatusOK)

		// 				Event{
		// 					ID:   "2",
		// 					Data: []byte("hello again"),
		// 				}.Write(w)

		// 				flusher.Flush()
		// 			},
		// 		)

		// 		err := source.Connect()
		// 		Ω(err).ShouldNot(HaveOccurred())
		// 		server.CloseClientConnections()
		// 	})

		// 	It("retries", func() {
		// 		Ω(source.Next()).Should(Equal(Event{
		// 			ID:   "2",
		// 			Data: []byte("hello again"),
		// 		}))
		// 	})
		// })
	})
})
