package api

import (
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/galactica-corp/guardians-sdk/pkg/zkcertificate"
)

const userDataStoringTime = 30 * time.Minute

func addCertToDB(db *badger.DB, userID UserID, cert []byte) error {
	return db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(userID), cert).WithTTL(userDataStoringTime)
		if err := txn.SetEntry(e); err != nil {
			return fmt.Errorf("failed to set certificate to db: %w", err)
		}
		return nil
	})
}

func readCertFromDB(db *badger.DB, userID UserID) (string, error) {
	var certData []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(userID))
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("certificate not found")
		}
		if err != nil {
			return fmt.Errorf("error retrieving certificate: %w", err)
		}
		return item.Value(func(val []byte) error {
			certData = append([]byte{}, val...)
			return nil
		})
	})
	if err != nil {
		return "", err
	}
	return string(certData), nil
}

func deleteUserDataFromDB(db *badger.DB, userID UserID) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(userID))
	})
}

func stripToSix(hash zkcertificate.Hash) string {
	s := hash.String()
	if len(s) > 6 {
		return s[:6]
	}
	return s
}
