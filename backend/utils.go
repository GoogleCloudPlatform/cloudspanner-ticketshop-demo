/*
Copyright 2018 Google Inc. All Rights Reserved.

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

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
)

var (
	alpha2continents = map[string]string{
		"Africa":        "AF",
		"Asia":          "AS",
		"Europe":        "EU",
		"Australia":     "OC",
		"North America": "NA",
		"South America": "SA",
		"Antarctica":    "AN",
	}
)

func readJSON(dst interface{}, file string) error {
	r, err := os.Open(file)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := json.NewDecoder(r).Decode(dst); err != nil {
		return err
	}
	return nil
}

type uuidHex string

func decodeHexUUIDStr(h uuidHex) (string, error) {
	b, err := hex.DecodeString(string(h))
	if err != nil {
		return "", err
	}
	var id = new(uuid.UUID)
	if err := id.UnmarshalBinary(b); err != nil {
		return "", err
	}
	return id.String(), nil
}

func encodeHexUUIDStr(id string) (uuidHex, error) {
	u, err := uuid.Parse(id)
	if err != nil {
		debugLog.Printf("Failed UUID string: %v", id)
		return "", err
	}

	b, err := u.MarshalBinary()
	if err != nil {
		return "", err
	}
	return uuidHex(hex.EncodeToString(b)), nil
}

func encodeHexUUID(id uuid.UUID) (uuidHex, error) {
	b, err := id.MarshalBinary()
	if err != nil {
		return "", err
	}

	return uuidHex(hex.EncodeToString(b)), nil
}

type customTicketBackendError struct {
	kind string
	s    string
}

func (e *customTicketBackendError) Error() string {
	return fmt.Sprintf("%s: %s", e.kind, e.s)
}

type debugging bool

// debugging log
func (d debugging) Printf(format string, args ...interface{}) {
	if d {
		log.Printf(format, args...)
	}
}
