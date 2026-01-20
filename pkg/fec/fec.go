package fec

import (
	"errors"

	"github.com/klauspost/reedsolomon"
)

// ErrIncomplete indicates that not enough shards are present yet to reconstruct the data
var ErrIncomplete = errors.New("not enough shards to reconstruct data")

// ErrUnrecoverable indicates that too many shards have been lost to ever reconstruct the data
var ErrUnrecoverable = errors.New("too many missing shards for Reed-Solomon reconstruction")

// FEC implements Forward Error Correction using Reed-Solomon codes
type FEC struct {
	dataShards   int
	parityShards int
	shardSize    int
	encoder      reedsolomon.Encoder
}

// NewFEC creates a new FEC encoder/decoder
// dataShards: number of data shards
// parityShards: number of parity shards for error correction
func NewFEC(dataShards, parityShards, shardSize int) (*FEC, error) {
	if dataShards <= 0 || parityShards <= 0 {
		return nil, errors.New("dataShards and parityShards must be positive")
	}
	if shardSize <= 0 {
		return nil, errors.New("shardSize must be positive")
	}

	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, err
	}

	return &FEC{
		dataShards:   dataShards,
		parityShards: parityShards,
		shardSize:    shardSize,
		encoder:      enc,
	}, nil
}

// Encode splits data into shards and generates parity shards
// Returns all shards (data + parity)
func (f *FEC) Encode(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}

	// Calculate padding needed
	totalShards := f.dataShards
	shardSize := (len(data) + totalShards - 1) / totalShards
	
	// Align to shardSize if specified
	if f.shardSize > 0 && shardSize < f.shardSize {
		shardSize = f.shardSize
	}

	// Create data shards
	shards := make([][]byte, f.dataShards+f.parityShards)
	for i := 0; i < f.dataShards; i++ {
		shards[i] = make([]byte, shardSize)
		start := i * shardSize
		end := start + shardSize
		if end > len(data) {
			end = len(data)
		}
		if start < len(data) {
			copy(shards[i], data[start:end])
		}
	}

	// Create parity shards using Reed-Solomon
	for i := 0; i < f.parityShards; i++ {
		shards[f.dataShards+i] = make([]byte, shardSize)
	}

	if err := f.encoder.Encode(shards); err != nil {
		return nil, err
	}

	return shards, nil
}

// Decode reconstructs data from shards (can handle missing shards if enough remain)
func (f *FEC) Decode(shards [][]byte, shardPresent []bool) ([]byte, error) {
	if len(shards) != f.dataShards+f.parityShards {
		return nil, errors.New("incorrect number of shards")
	}
	if len(shardPresent) != len(shards) {
		return nil, errors.New("shardPresent length mismatch")
	}

	// Count present shards
	presentCount := 0
	for _, present := range shardPresent {
		if present {
			presentCount++
		}
	}

	if presentCount < f.dataShards {
		return nil, ErrIncomplete
	}

	// Determine shard size from any available shard and validate consistency
	var shardSize int
	for i := 0; i < len(shards); i++ {
		if shardPresent[i] && shards[i] != nil && len(shards[i]) > 0 {
			shardSize = len(shards[i])
			break
		}
	}
	if shardSize == 0 {
		return nil, errors.New("no valid shards found to determine shard size")
	}
	for i := 0; i < len(shards); i++ {
		if shardPresent[i] && shards[i] != nil && len(shards[i]) != shardSize {
			return nil, errors.New("inconsistent shard size")
		}
	}

	// Mark missing shards as nil for reconstruction
	for i := 0; i < len(shards); i++ {
		if !shardPresent[i] {
			shards[i] = nil
		}
	}

	if err := f.encoder.Reconstruct(shards); err != nil {
		if presentCount == f.dataShards+f.parityShards {
			return nil, ErrUnrecoverable
		}
		return nil, ErrIncomplete
	}

	// Reconstruct original data
	result := make([]byte, 0, f.dataShards*shardSize)
	for i := 0; i < f.dataShards; i++ {
		result = append(result, shards[i]...)
	}

	return result, nil
}

// DataShards returns the number of data shards
func (f *FEC) DataShards() int {
	return f.dataShards
}

// ParityShards returns the number of parity shards
func (f *FEC) ParityShards() int {
	return f.parityShards
}

// TotalShards returns the total number of shards
func (f *FEC) TotalShards() int {
	return f.dataShards + f.parityShards
}

// EncodeShards encodes data+parity shards using Reed-Solomon.
// shards length must be dataShards+parityShards and each shard must be the same size.
func EncodeShards(shards [][]byte, dataShards, parityShards int) error {
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return err
	}
	return enc.Encode(shards)
}

// ReconstructShards reconstructs missing shards in-place.
// Missing shards should be nil.
func ReconstructShards(shards [][]byte, dataShards, parityShards int) error {
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return err
	}
	return enc.Reconstruct(shards)
}
