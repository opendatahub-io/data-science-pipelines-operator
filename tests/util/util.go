package util

import (
	"fmt"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
)

func get(path string) string {
	response, err := http.Get(path)
	Expect(err).ToNot(HaveOccurred())

	responseData, err := ioutil.ReadAll(response.Body)
	Expect(err).ToNot(HaveOccurred())
	return string(responseData)
}

func get_pipelines(url string) string {
	return get(fmt.Sprintf("%s/apis/v1beta1/pipelines/upload", url))
}

func get_experiments(url string) string {
	return get(fmt.Sprintf("%s/apis/v1beta1/experiments", url))
}
