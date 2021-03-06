package http_crawler_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	. "github.com/alphagov/govuk_crawler_worker/http_crawler"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testServer(status int, body string) *httptest.Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprintln(w, body)
	}
	return httptest.NewServer(http.HandlerFunc(handler))
}

var _ = Describe("Crawl", func() {
	var crawler *Crawler
	var rootURLs []*url.URL
	var urlA, urlB *url.URL
	var token string

	BeforeEach(func() {
		urlA = &url.URL{
			Scheme: "http",
			Host:   "127.0.0.1",
			Path:   "/",
		}
		urlB = &url.URL{
			Scheme: "http",
			Host:   "127.0.0.2",
			Path:   "/",
		}
		rootURLs = []*url.URL{urlA, urlB}
		token = "Ay8aix8guitai0uud4ohdeiqu0theuyeiy3Da1ool6nau0ohphaey9nai5teeDac"
		crawler = NewCrawler(rootURLs, "0.0.0", token, nil)
		Expect(crawler).ToNot(BeNil())
	})

	Describe("NewCrawler()", func() {
		It("can accept username and password for HTTP Basic Auth", func() {
			// Returns a HandlerFunc that authenticates via Basic
			// Auth. Writes a http.StatusUnauthorized if
			// authentication fails.
			basic := func(username string, password string) http.HandlerFunc {
				unauthorized := func(res http.ResponseWriter) {
					res.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
					http.Error(res, "Not Authorized", http.StatusUnauthorized)
				}
				siteAuth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

				return func(res http.ResponseWriter, req *http.Request) {
					if req.Header.Get("Authorization") != ("Basic " + siteAuth) {
						unauthorized(res)
						return
					}

					res.WriteHeader(http.StatusOK)
					res.Write([]byte("You've successfully logged in with basic auth!"))
				}
			}

			basicAuthTestServer := httptest.NewServer(http.HandlerFunc(basic("username", "password")))
			defer basicAuthTestServer.Close()

			basicAuthCrawler := NewCrawler([]*url.URL{urlA}, "0.0.0", token, &BasicAuth{"username", "password"})

			testURL, _ := url.Parse(basicAuthTestServer.URL)
			response, err := basicAuthCrawler.Crawl(testURL)

			Expect(err).To(BeNil())
			Expect(string(response.Body)).To(Equal("You've successfully logged in with basic auth!"))
		})
	})

	Describe("Crawler.Crawl()", func() {
		It("specifies a user agent when making a request", func() {
			userAgentTestServer := func(httpStatus int) *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(httpStatus)
					fmt.Fprintln(w, r.UserAgent())
				}))
			}

			ts := userAgentTestServer(http.StatusOK)
			defer ts.Close()

			testURL, _ := url.Parse(ts.URL)
			response, err := crawler.Crawl(testURL)

			Expect(err).To(BeNil())
			Expect(string(response.Body)).Should(MatchRegexp("GOV.UK Crawler Worker/" + "0.0.0"))
		})

		It("returns an error when a redirect is encounted", func() {
			redirectTestServer := func(httpStatus int) *httptest.Server {
				return httptest.NewServer(http.RedirectHandler("/redirect", httpStatus))
			}

			ts := redirectTestServer(http.StatusMovedPermanently)
			defer ts.Close()

			testURL, _ := url.Parse(ts.URL)
			_, err := crawler.Crawl(testURL)

			Expect(err).To(Equal(errors.New("HTTP redirect encountered")))
		})

		It("returns an error when server returns a 404", func() {
			ts := testServer(http.StatusNotFound, "Nothing to see here")
			defer ts.Close()

			testURL, _ := url.Parse(ts.URL)
			_, err := crawler.Crawl(testURL)

			Expect(err).To(Equal(errors.New("404 Not Found")))
		})

		It("returns a body with no errors for 200 OK responses", func() {
			ts := testServer(http.StatusOK, "Hello world")
			defer ts.Close()

			testURL, _ := url.Parse(ts.URL)
			response, err := crawler.Crawl(testURL)

			Expect(err).To(BeNil())
			Expect(strings.TrimSpace(string(response.Body))).To(Equal("Hello world"))
		})

		It("returns an error if URL host is not in rootURLs", func() {
			testURL, _ := url.Parse("http://www.google.com/foo")
			response, err := crawler.Crawl(testURL)

			Expect(err).To(Equal(ErrCannotCrawlURL))
			Expect(response).To(BeNil())
		})

		Describe("returning a retry error", func() {
			It("returns a retry error if we get a response code of Too Many Requests", func() {
				ts := testServer(429, "Too Many Requests")
				defer ts.Close()

				testURL, _ := url.Parse(ts.URL)
				response, err := crawler.Crawl(testURL)

				Expect(err).To(Equal(ErrRetryRequest429))
				Expect(response).To(BeNil())
			})

			It("returns a retry error if we get a response code of Internal Server Error", func() {
				ts := testServer(http.StatusInternalServerError, "Internal Server Error")
				defer ts.Close()

				testURL, _ := url.Parse(ts.URL)
				response, err := crawler.Crawl(testURL)

				Expect(err).To(Equal(ErrRetryRequest5XX))
				Expect(response).To(BeNil())
			})

			It("returns a retry error if we get a response code of Gateway Timeout", func() {
				ts := testServer(http.StatusGatewayTimeout, "Gateway Timeout")
				defer ts.Close()

				testURL, _ := url.Parse(ts.URL)
				response, err := crawler.Crawl(testURL)

				Expect(err).To(Equal(ErrRetryRequest5XX))
				Expect(response).To(BeNil())
			})
		})
	})

	Describe("RetryStatusCodes", func() {
		It("should return a fixed int array with values 429, 500..599", func() {
			statusCodes := Retry5XXStatusCodes()

			Expect(len(statusCodes)).To(Equal(100))
			Expect(statusCodes[0]).To(Equal(500))
			Expect(statusCodes[99]).To(Equal(599))
		})
	})

	Describe("IsAllowedHost", func() {
		It("should return true if URL has allowed host", func() {
			ret := IsAllowedHost(urlA.Host, []*url.URL{urlA})
			Expect(ret).To(BeTrue())
		})

		It("should return false if URL host is not allowed", func() {
			ret := IsAllowedHost(urlB.Host, []*url.URL{urlA})
			Expect(ret).To(BeFalse())
		})
	})

	Describe("HostOnly", func() {
		It("should return only the host from a host:port tuplet", func() {
			ret, err := HostOnly("foo:8443")

			Expect(ret).To(Equal("foo"))
			Expect(err).To(BeNil())
		})

		It("should return the hostname if no port is specified", func() {
			ret, err := HostOnly("foo")

			Expect(ret).To(Equal("foo"))
			Expect(err).To(BeNil())
		})
	})
})
