package region

import (
	"testing"

	"github.com/NetSys/quilt/db"
)

func TestSetDefault(t *testing.T) {
	exp := "foo"
	m := db.Machine{Provider: "Amazon", Region: exp}
	m = SetDefault(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m = SetDefault(m)
	exp = "us-west-1"
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "Google"
	exp = "us-east1-b"
	m = SetDefault(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "Vagrant"
	exp = ""
	m = SetDefault(m)
	if m.Region != exp {
		t.Errorf("expected %s, found %s", exp, m.Region)
	}

	m.Region = ""
	m.Provider = "Panic"
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic")
		}
	}()

	m = SetDefault(m)
}

func TestDefault(t *testing.T) {
	r := Default("Amazon")
	exp := "us-west-1"
	if r != exp {
		t.Errorf("expected %s, found %s", exp, r)
	}

	exp = "us-east1-b"
	r = Default("Google")
	if r != exp {
		t.Errorf("expected %s, found %s", exp, r)
	}

	exp = ""
	r = Default("Vagrant")
	if r != exp {
		t.Errorf("expected %s, found %s", exp, r)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic")
		}
	}()

	Default("Panic")
}
