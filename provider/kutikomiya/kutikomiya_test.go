package kutikomiya

import (
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestKutikomiya_GetActorInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"早坂咲重",
	})
}

func TestKutikomiya_SearchActor(t *testing.T) {
	testkit.Test(t, New, []string{
		"早坂咲重",
	})
}