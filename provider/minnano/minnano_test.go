package minnano

import (
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestMinnano_GetActorInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"356473",
	})
}

func TestMinnano_SearchActor(t *testing.T) {
	testkit.Test(t, New, []string{
		"鈴音りおな",
		"鈴木さとみ",
	})
}