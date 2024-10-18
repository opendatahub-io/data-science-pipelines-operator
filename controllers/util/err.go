/*
Copyright 2024.

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

package util

import (
	"fmt"
)

// LaggingDependencyCreationError should be used if a dependency that is
// created by a third party is not found (e.g. service-ca secrets).
type LaggingDependencyCreationError struct {
	Message string
}

func (e *LaggingDependencyCreationError) Error() string {
	return fmt.Sprintf("Missing dependency error: %s", e.Message)
}
