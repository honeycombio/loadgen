package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func Test_PeriodicEligibility_checkEligible(t *testing.T) {
	words := strings.Split("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", "")
	notFound := map[string]struct{}{}
	for _, w := range words {
		notFound[w] = struct{}{}
	}

	pe := newPeriodicEligibility(NewRng("hello"), words, 60*time.Second)
	t.Run("only some words show up for short period", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			for p := 0; p < 30; p++ {
				word := pe.getEligibleWord(time.Duration(p) * time.Second)
				delete(notFound, word)
			}
			if len(notFound) == 0 {
				break
			}
		}
		if len(notFound) != 0 {
			t.Errorf("expected some words to be not found, got none")
		}
	})

	t.Run("all eligible words show up with full period", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			for p := 0; p < 60; p++ {
				word := pe.getEligibleWord(time.Duration(p) * time.Second)
				delete(notFound, word)
			}
			if len(notFound) == 0 {
				break
			}
		}
		if len(notFound) > 0 {
			t.Errorf("expected all words to be found, got %v", notFound)
		}
	})
}

func BenchmarkPeriodicEligibility(b *testing.B) {
	for _, card := range []int{10, 50, 200} {
		var words []string
		for i := 0; i < card; i++ {
			words = append(words, strconv.Itoa(i))
		}
		period := 61 * time.Second
		pe := newPeriodicEligibility(NewRng("hello"), words, period)
		for p := 0; p < 61; p += 10 {
			b.Run(fmt.Sprintf("card_%02d_p_%02d", card, p), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					pe.getEligibleWord(time.Duration(p) * time.Second)
				}
			})
		}
	}
}
