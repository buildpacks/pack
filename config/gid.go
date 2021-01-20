package config

import (
	"strconv"

	"github.com/pkg/errors"
)

type GroupID struct {
	ID           string
	FlagProvided bool
}

func (gid *GroupID) GetID(sGID string) (int, error) {
	if gid.FlagProvided {
		id, err := strconv.Atoi(gid.ID)

		if err != nil {
			return 0, errors.Errorf("invalid gid %s", gid.ID)
		}

		return id, nil
	}

	return strconv.Atoi(sGID)
}
