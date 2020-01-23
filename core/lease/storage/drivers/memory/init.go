package memory

import "github.com/nextdhcp/nextdhcp/core/lease/storage"

func init() {
	storage.MustRegister("memory", func(_ map[string][]string) (storage.LeaseStorage, error) {
		memory := makeStorage()
		return memory, nil
	})
}
