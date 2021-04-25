package web

import (
	"io/ioutil"
	"net/http"
)

// MustGet will retrieve a url and return the body of the page.
// If Get encounters any errors, it will panic.
func MustGet(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}

	// don't forget to close the body
	defer resp.Body.Close()
	var body []byte
	if body, err = ioutil.ReadAll(resp.Body); err != nil {
		panic(err)
	}
	return string(body)
}
