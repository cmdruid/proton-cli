package pass

import (
	"github.com/ProtonMail/gopenpgp/v2/helper"
)

// Proton Pass encrypts an export to a passphrase rather than to a key, so a
// backup can be opened on a machine that has never held the account's keys.
//
// The work factor is the highest the format offers, because the file is meant to
// leave the machine and a passphrase somebody can remember is all that stands in
// front of it.
func encryptExport(plain []byte, passphrase string) (string, error) {
	return helper.EncryptMessageWithPassword([]byte(passphrase), string(plain))
}

func decryptExport(armored, passphrase string) ([]byte, error) {
	out, err := helper.DecryptMessageWithPassword([]byte(passphrase), armored)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}
