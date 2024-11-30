package main

import (
	"testing"
)

func TestParseTransactionLine(t *testing.T) {
	line := "transaction 6374, UTC time 2024-11-30T11:45:36.870201"
	// number, timestamp, err := parseTransactionLine(line)
	number, err := parseTransactionLine(line)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedNumber := 6374
	// expectedTime := time.Date(2024, 11, 30, 11, 45, 36, 870201000, time.UTC).Unix()

	if number != expectedNumber {
		t.Errorf("Expected transaction number %d, got %d", expectedNumber, number)
	}

	/* if timestamp != expectedTime {
		t.Errorf("Expected timestamp %d, got %d", expectedTime, timestamp)
	} */
}

