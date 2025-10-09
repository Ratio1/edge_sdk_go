package ratio1_sdk

import (
	"fmt"

	"github.com/Ratio1/ratio1_sdk_go/pkg/cstore"
	"github.com/Ratio1/ratio1_sdk_go/pkg/r1fs"
)

// NewFromEnv initialises CStore and R1FS clients using the per-package helpers
// and returns the resolved runtime mode ("http" or "mock").
func NewFromEnv() (*cstore.Client, *r1fs.Client, string, error) {
	cs, cMode, err := cstore.NewFromEnv()
	if err != nil {
		return nil, nil, "", fmt.Errorf("ratio1_sdk: init cstore client: %w", err)
	}

	fs, fMode, err := r1fs.NewFromEnv()
	if err != nil {
		return nil, nil, "", fmt.Errorf("ratio1_sdk: init r1fs client: %w", err)
	}

	mode := cMode
	if mode == "" {
		mode = fMode
	}
	if fMode != "" && mode != "" && fMode != mode {
		return nil, nil, "", fmt.Errorf("ratio1_sdk: mismatched runtime modes (cstore=%s, r1fs=%s)", cMode, fMode)
	}

	return cs, fs, mode, nil
}
