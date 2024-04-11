// Copyright © 2024 Meroxa, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cchan

import (
	"context"
	"fmt"
	"time"
)

func ExampleSelect() {
	ctx := context.Background()
	ch0 := make(chan int)
	ch1 := make(chan int)
	ch2 := make(chan int)

	go func() {
		ch0 <- 1
		ch1 <- 2
		close(ch0)
		time.Sleep(time.Millisecond) // give time for the closed channel to be detected
		ch2 <- 3
		close(ch2)
		time.Sleep(time.Millisecond) // give time for the closed channel to be detected
		close(ch1)
	}()

	s := NewSelect(ch0, ch1, ch2)
	for s.HasMore() {
		selected, err := s.Select(ctx)
		if err != nil {
			// context error
			break
		}
		if !selected.Ok {
			// channel closed
			fmt.Printf("ch%d closed\n", selected.Index)
			continue
		}
		fmt.Printf("ch%d: %v\n", selected.Index, selected.Value)
	}

	// no more values to read, Select will block until the context is canceled
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	_, err := s.Select(ctx)
	fmt.Println(err)

	// Output:
	// ch0: 1
	// ch1: 2
	// ch0 closed
	// ch2: 3
	// ch2 closed
	// ch1 closed
	// context deadline exceeded
}
