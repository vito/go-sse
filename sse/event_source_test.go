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

		source = &EventSource{
			Client:               http.DefaultClient,
			DefaultRetryInterval: 100 * time.Millisecond,
			CreateRequest: func() *http.Request {
				request, err := http.NewRequest("GET", server.URL(), nil)
				Ω(err).ShouldNot(HaveOccurred())

				return request
			},
		}
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
})
