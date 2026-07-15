package secrets

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func FuzzLoadKeyringJSON(f *testing.F) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	f.Add([]byte(fmt.Sprintf(`{"active":"kek-1","keys":{"kek-1":"%s"}}`, key)))
	f.Add([]byte(`{"active":"missing","keys":{}}`))
	f.Add([]byte(`not json`))

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > maxKeyringBytes+1 {
			return
		}
		keyring, err := LoadKeyringJSON(input)
		if err != nil {
			return
		}
		if keyring.ActiveID() == "" || !keyring.Has(keyring.ActiveID()) {
			t.Fatal("accepted keyring has no usable active key")
		}
	})
}
