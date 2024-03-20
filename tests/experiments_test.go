//go:build test_integration

/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func (suite *IntegrationTestSuite) TestFetchExperiments() {
	suite.T().Run("Should successfully fetch experiments", func(t *testing.T) {
		response, err := http.Get(fmt.Sprintf("%s/apis/v2beta1/experiments", APIServerURL))
		require.NoError(t, err, "Error fetching experiments")

		responseData, err := ioutil.ReadAll(response.Body)
		defer response.Body.Close()
		require.NoError(t, err, "Error reading response body")

		suite.Assert().Equal(200, response.StatusCode, "Expected HTTP status code 200 for fetching experiments")
		loggr.Info(string(responseData))
	})
}
