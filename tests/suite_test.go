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

package systemtests

import (
	"context"
	"go.uber.org/zap/zapcore"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	timeout  = time.Second * 6
	interval = time.Millisecond * 2
)

// TestAPIs - This is the entry point for Ginkgo -
// the go test runner will run this function when you run go test or ginkgo.
// Under the hood, Ginkgo is simply calling go test.
// You can run go test instead of the ginkgo CLI, But Ginkgo has several capabilities that can only be accessed via ginkgo.
// It is best practice to embrace the ginkgo CLI and treat it as a first-class member of the testing toolchain.
func TestAPIs(t *testing.T) {
	// Single line of glue code connecting Ginkgo to Gomega
	// Inform our matcher library (Gomega) which function to call (Ginkgo's Fail) in the event a failure is detected.
	RegisterFailHandler(Fail)

	// Inform Ginkgo to start the test suite, passing it the *testing.T instance and a description of the suite.
	// Only call RunSpecs once and let Ginkgo worry about calling *testing.T for us.
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())

	// Initialize logger
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.TimeEncoderOfLayout(time.RFC3339),
	}
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseFlagOptions(&opts)))

})

var _ = AfterSuite(func() {

})
