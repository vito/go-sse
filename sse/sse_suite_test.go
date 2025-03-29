package sse_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestSSE(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SSE Suite")
}
