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
	"reflect"
	"sort"
)

// Select is a utility for selecting from multiple channels. It is similar to
// the select statement, but it can handle a dynamic number of channels.
//
// The Select type is not safe for concurrent use. If you need to use it
// concurrently, you should synchronize access to it.
//
// Example:
//
//	ch1 := make(chan int)
//	ch2 := make(chan int)
//	s := NewSelect(ch1, ch2)
//	for s.HasMore() {
//	  selected, err := s.Select(context.Background())
//	  if err != nil {
//	    // context error
//	    break
//	  }
//	  if !selected.Ok {
//	    // channel closed
//	    continue
//	  }
//	  switch selected.Index {
//	  case 1:
//	    fmt.Println("ch1:", selected.Value)
//	  case 2:
//	    fmt.Println("ch2:", selected.Value)
//	  }
//	}
type Select[T any] struct {
	channels   []<-chan T
	closed     []int
	selectFunc func(ctx context.Context) (int, T, bool)

	zeroT T // used for returning a zero value
}

// NewSelect creates a new Select instance with the given channels.
func NewSelect[T any](channels ...<-chan T) *Select[T] {
	s := Select[T]{channels: channels}
	s.updateSelectFunc()
	return &s
}

// Selected is the result of a select operation.
type Selected[T any] struct {
	// Index is the index of the selected channel. If the context was canceled,
	// it will be -1.
	Index int
	// Value is the value received from the selected channel. If the context was
	// canceled or the channel was closed, it will be the zero value of T.
	Value T
	// Ok is true if the value was received from the channel. If the context was
	// canceled or the channel was closed, it will be false.
	Ok bool
}

// OpenChannels returns the open channels that are still part of the select
// case. If all channels are closed, it will return nil.
// Note that channels in the returned slice are not necessarily open, they
// are just the channels that are still part of the select case (potentially open).
func (s *Select[T]) OpenChannels() []<-chan T {
	if len(s.channels) == len(s.closed) {
		return nil
	}
	c := make([]<-chan T, len(s.channels)-len(s.closed))
	j := 0
	for i, ch := range s.channels {
		if j < len(s.closed) && i == s.closed[j] {
			j++
			continue
		}
		c[i-j] = ch
	}
	return c
}

// ClosedChannels returns the closed channels that were removed from the select
// case. If no channels are closed, it will return nil.
func (s *Select[T]) ClosedChannels() []<-chan T {
	if len(s.closed) == 0 {
		return nil
	}
	c := make([]<-chan T, len(s.closed))
	for i, j := range s.closed {
		c[i] = s.channels[j]
	}
	return c
}

// HasMore returns true if there are more channels to select from. If all
// channels are closed, it will return false.
func (s *Select[T]) HasMore() bool {
	return len(s.channels) > len(s.closed)
}

// Select selects a value from the channels. If a channel is closed, it will
// be removed from the select case. If the context is canceled, it will return
// the context error.
// Note that running Select after HasMore has returned false will always block
// until the context is canceled, same as a select statement with a single case
// that receives from ctx.Done().
func (s *Select[T]) Select(ctx context.Context) (Selected[T], error) {
	var sl Selected[T]
	sl.Index, sl.Value, sl.Ok = s.selectFunc(ctx)

	// Adjust index according to indexes of closed channels.
	if sl.Index >= 0 && len(s.closed) > 0 {
		for _, i := range s.closed {
			if sl.Index >= i {
				sl.Index++
			}
		}
	}

	if !sl.Ok {
		if sl.Index == -1 {
			// Context was canceled.
			return sl, ctx.Err()
		}
		// One of the channels is closed, remove it from select case.
		s.closed = append(s.closed, sl.Index)
		sort.Ints(s.closed)
		s.updateSelectFunc()
	}

	return sl, nil
}

// updateSelectFunc updates s.selectFunc based on the number of open channels.
func (s *Select[T]) updateSelectFunc() {
	in := s.OpenChannels()

	switch len(in) {
	case 0:
		s.selectFunc = func(ctx context.Context) (int, T, bool) {
			<-ctx.Done()
			return -1, s.zeroT, false
		}
	case 1:
		s.selectFunc = func(ctx context.Context) (int, T, bool) { return s.select1(ctx, in[0]) }
	case 2:
		s.selectFunc = func(ctx context.Context) (int, T, bool) { return s.select2(ctx, in[0], in[1]) }
	case 3:
		s.selectFunc = func(ctx context.Context) (int, T, bool) {
			return s.select3(ctx, in[0], in[1], in[2])
		}
	case 4:
		s.selectFunc = func(ctx context.Context) (int, T, bool) {
			return s.select4(ctx, in[0], in[1], in[2], in[3])
		}
	case 5:
		s.selectFunc = func(ctx context.Context) (int, T, bool) {
			return s.select5(ctx, in[0], in[1], in[2], in[3], in[4])
		}
	case 6:
		s.selectFunc = func(ctx context.Context) (int, T, bool) {
			return s.select6(ctx, in[0], in[1], in[2], in[3], in[4], in[5])
		}
	default:
		// use reflection for more channels
		cases := make([]reflect.SelectCase, len(in)+1)
		for i, ch := range in {
			cases[i+1] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch)}
		}
		s.selectFunc = func(ctx context.Context) (int, T, bool) {
			cases[0] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}
			chosen, value, ok := reflect.Select(cases)
			if !ok { // a channel was closed
				return chosen - 1, s.zeroT, ok
			}
			return chosen - 1, value.Interface().(T), ok
		}
	}
}

func (s *Select[T]) select1(
	ctx context.Context,
	c1 <-chan T,
) (int, T, bool) {
	select {
	case <-ctx.Done():
		return -1, s.zeroT, false
	case val, ok := <-c1:
		return 0, val, ok
	}
}

func (s *Select[T]) select2(
	ctx context.Context,
	c1 <-chan T,
	c2 <-chan T,
) (int, T, bool) {
	select {
	case <-ctx.Done():
		return -1, s.zeroT, false
	case val, ok := <-c1:
		return 0, val, ok
	case val, ok := <-c2:
		return 1, val, ok
	}
}

func (s *Select[T]) select3(
	ctx context.Context,
	c1 <-chan T,
	c2 <-chan T,
	c3 <-chan T,
) (int, T, bool) {
	select {
	case <-ctx.Done():
		return -1, s.zeroT, false
	case val, ok := <-c1:
		return 0, val, ok
	case val, ok := <-c2:
		return 1, val, ok
	case val, ok := <-c3:
		return 2, val, ok
	}
}

func (s *Select[T]) select4(
	ctx context.Context,
	c1 <-chan T,
	c2 <-chan T,
	c3 <-chan T,
	c4 <-chan T,
) (int, T, bool) {
	select {
	case <-ctx.Done():
		return -1, s.zeroT, false
	case val, ok := <-c1:
		return 0, val, ok
	case val, ok := <-c2:
		return 1, val, ok
	case val, ok := <-c3:
		return 2, val, ok
	case val, ok := <-c4:
		return 3, val, ok
	}
}

func (s *Select[T]) select5(
	ctx context.Context,
	c1 <-chan T,
	c2 <-chan T,
	c3 <-chan T,
	c4 <-chan T,
	c5 <-chan T,
) (int, T, bool) {
	select {
	case <-ctx.Done():
		return -1, s.zeroT, false
	case val, ok := <-c1:
		return 0, val, ok
	case val, ok := <-c2:
		return 1, val, ok
	case val, ok := <-c3:
		return 2, val, ok
	case val, ok := <-c4:
		return 3, val, ok
	case val, ok := <-c5:
		return 4, val, ok
	}
}

func (s *Select[T]) select6(
	ctx context.Context,
	c1 <-chan T,
	c2 <-chan T,
	c3 <-chan T,
	c4 <-chan T,
	c5 <-chan T,
	c6 <-chan T,
) (int, T, bool) {
	select {
	case <-ctx.Done():
		return -1, s.zeroT, false
	case val, ok := <-c1:
		return 0, val, ok
	case val, ok := <-c2:
		return 1, val, ok
	case val, ok := <-c3:
		return 2, val, ok
	case val, ok := <-c4:
		return 3, val, ok
	case val, ok := <-c5:
		return 4, val, ok
	case val, ok := <-c6:
		return 5, val, ok
	}
}
