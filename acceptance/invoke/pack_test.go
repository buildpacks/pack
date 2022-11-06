//go:build acceptance
// +build acceptance

package invoke

import (
	"io/ioutil"
	"testing"

	"github.com/buildpacks/pack/testhelpers"
)

func TestPack(t *testing.T) {
	assert := testhelpers.NewAssertionManager(t)

	t.Run("hasFlag:exists", func(t *testing.T) {
		packBuildHelp, err := ioutil.ReadFile("testdata/pack_build_help.txt")
		assert.Nil(err)
		assert.Equal(hasFlag(string(packBuildHelp), "--quiet"), true)
	})

	t.Run("hasFlag:non-existent", func(t *testing.T) {
		packBuildHelp, err := ioutil.ReadFile("testdata/pack_build_help.txt")
		assert.Nil(err)
		assert.Equal(hasFlag(string(packBuildHelp), "--non-existent"), false)
	})

	t.Run("hasCommand:exists", func(t *testing.T) {
		packHelp, err := ioutil.ReadFile("testdata/pack_help.txt")
		assert.Nil(err)
		assert.Equal(hasCommand(string(packHelp), "build"), true)
	})

	t.Run("hasCommand:non-existent", func(t *testing.T) {
		packHelp, err := ioutil.ReadFile("testdata/pack_help.txt")
		assert.Nil(err)
		// we use a word that is not in the help text to
		// make sure we're not just getting lucky
		assert.Equal(hasCommand(string(packHelp), "building"), false)
	})
}
