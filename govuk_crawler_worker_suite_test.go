package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestGovukCrawlerWorker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GovukCrawlerWorker Suite")
}
