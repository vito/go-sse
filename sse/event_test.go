package sse_test

import (
	. "github.com/vito/go-sse/sse"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Event", func() {
	Describe("Encode", func() {
		It("encodes to a dispatchable event", func() {
			Ω(Event{
				ID:   "some-id",
				Name: "some-name",
				Data: []byte("some-data"),
			}.Encode()).Should(Equal("id: some-id\nevent: some-name\ndata: some-data\n\n"))
		})

		It("splits lines across multiple data segments", func() {
			Ω(Event{
				ID:   "some-id",
				Name: "some-name",
				Data: []byte("some-data\nsome-more-data\n"),
			}.Encode()).Should(Equal("id: some-id\nevent: some-name\ndata: some-data\ndata: some-more-data\ndata\n\n"))
		})
	})
})
