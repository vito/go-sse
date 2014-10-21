package sse_test

import (
	"bytes"
	"io"
	"strings"

	. "github.com/vito/go-sse/sse"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Reader", func() {
	var (
		eventStream string

		buffer *bytes.Buffer

		reader *Reader
	)

	BeforeEach(func() {
		eventStream = ""

		buffer = new(bytes.Buffer)

		reader = NewReader(buffer)
	})

	JustBeforeEach(func() {
		_, err := buffer.Write([]byte(eventStream))
		Ω(err).ShouldNot(HaveOccurred())
	})

	Context("when a comment appears on the stream", func() {
		BeforeEach(func() {
			eventStream += ":foo bar baz\n"
		})

		It("returns EOF", func() {
			_, err := reader.Next()
			Ω(err).Should(Equal(io.EOF))
		})

		Context("followed by an event", func() {
			BeforeEach(func() {
				eventStream += "data: hello\n\n"
			})

			It("returns event, skipping the comment", func() {
				event, err := reader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(event).Should(Equal(Event{Data: []byte("hello")}))
			})
		})
	})

	Context("when a sufficiently large data lake appears", func() {
		BeforeEach(func() {
			eventStream += "data: " + strings.Repeat("x", 8192) + "\n\n"
		})

		It("properly reads in the full string before emitting the event", func() {
			event, err := reader.Next()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(event).Should(Equal(Event{Data: []byte(strings.Repeat("x", 8192))}))
		})
	})

	Context("when CRLN is used as a line ending", func() {
		BeforeEach(func() {
			eventStream += ":foo bar baz\r\nid: 123\r\nevent: some-event\r\ndata: hello\r\n\r\n"
		})

		It("properly splits on it", func() {
			event, err := reader.Next()
			Ω(err).ShouldNot(HaveOccurred())
			Ω(event).Should(Equal(Event{
				ID:   "123",
				Name: "some-event",
				Data: []byte("hello"),
			}))
		})
	})

	Context("when an event comes on the stream", func() {
		Context("when an event id specified", func() {
			BeforeEach(func() {
				eventStream += `id: 12
event: some-event
data: hello

`
			})

			It("decodes an event into the given data structure", func() {
				event, err := reader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(event).Should(Equal(Event{
					ID:   "12",
					Name: "some-event",
					Data: []byte("hello"),
				}))
			})

			Context("and a second event arrives", func() {
				JustBeforeEach(func() {
					// skip first event
					_, err := reader.Next()
					Ω(err).ShouldNot(HaveOccurred())
				})

				Context("with a different id", func() {
					BeforeEach(func() {
						eventStream += `id: 13
event: some-other-event
data: hello again

`
					})

					It("returns an event with the new id", func() {
						event, err := reader.Next()
						Ω(err).ShouldNot(HaveOccurred())
						Ω(event).Should(Equal(Event{
							ID:   "13",
							Name: "some-other-event",
							Data: []byte("hello again"),
						}))
					})
				})

				Context("with no id specified", func() {
					BeforeEach(func() {
						eventStream += `event: some-other-event
data: hello again

`
					})

					It("returns an event with the same id as the previous event", func() {
						event, err := reader.Next()
						Ω(err).ShouldNot(HaveOccurred())
						Ω(event).Should(Equal(Event{
							ID:   "12",
							Name: "some-other-event",
							Data: []byte("hello again"),
						}))
					})
				})

				Context("with an empty id specified", func() {
					BeforeEach(func() {
						eventStream += `event: some-other-event
data: hello again
id

`
					})

					It("returns an event with an empty id", func() {
						event, err := reader.Next()
						Ω(err).ShouldNot(HaveOccurred())
						Ω(event).Should(Equal(Event{
							ID:   "",
							Name: "some-other-event",
							Data: []byte("hello again"),
						}))
					})
				})
			})
		})

		Context("but is not properly terminated", func() {
			BeforeEach(func() {
				eventStream += `id: 12
event: some-event
data: some-valuable-data
`
			})

			It("is not dispatched", func() {
				_, err := reader.Next()
				Ω(err).Should(Equal(io.EOF))
			})
		})

		Context("with no data", func() {
			BeforeEach(func() {
				eventStream += `id: 12
event: some-event

`
			})

			It("is not dispatched", func() {
				_, err := reader.Next()
				Ω(err).Should(Equal(io.EOF))
			})
		})

		Context("with multiple data fields", func() {
			BeforeEach(func() {
				eventStream += `id: 12
event: some-event
data: some-valuable-data
data: some-more-data

`
			})

			It("joins the two data fields with a linebreak", func() {
				event, err := reader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(event).Should(Equal(Event{
					ID:   "12",
					Name: "some-event",
					Data: []byte("some-valuable-data\nsome-more-data"),
				}))
			})
		})

		Context("with no spaces after the field names", func() {
			BeforeEach(func() {
				eventStream += `id:12
event:some-event
data:some-valuable-data

`
			})

			It("parses it properly", func() {
				event, err := reader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(event).Should(Equal(Event{
					ID:   "12",
					Name: "some-event",
					Data: []byte("some-valuable-data"),
				}))
			})
		})

		Context("with extra spaces after the field names", func() {
			BeforeEach(func() {
				eventStream += `id:  12
event:   some-event
data:    some-valuable-data

`
			})

			It("only removes the first space", func() {
				event, err := reader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(event).Should(Equal(Event{
					ID:   " 12",
					Name: "  some-event",
					Data: []byte("   some-valuable-data"),
				}))
			})
		})

		Context("with no field value", func() {
			BeforeEach(func() {
				eventStream += `id: 12
event: some-event
data
data

`
			})

			It("treats the full line as the name, with an empty value", func() {
				event, err := reader.Next()
				Ω(err).ShouldNot(HaveOccurred())
				Ω(event).Should(Equal(Event{
					ID:   "12",
					Name: "some-event",
					Data: []byte("\n"),
				}))
			})
		})
	})
})
