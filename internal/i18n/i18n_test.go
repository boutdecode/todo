package i18n

import "testing"

func TestCatalogsHaveSameKeys(t *testing.T) {
	for k := range en {
		if _, ok := fr[k]; !ok {
			t.Errorf("fr is missing %q", k)
		}
	}
	for k := range fr {
		if _, ok := en[k]; !ok {
			t.Errorf("en is missing %q", k)
		}
	}
}

func TestDetect(t *testing.T) {
	clear := func() {
		for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
			t.Setenv(v, "")
		}
	}
	clear()
	if Detect() != EN {
		t.Fatal("default must be en")
	}
	t.Setenv("LANG", "fr_FR.UTF-8")
	if Detect() != FR {
		t.Fatal("LANG fr")
	}
	t.Setenv("LC_ALL", "en_US.UTF-8")
	if Detect() != EN {
		t.Fatal("LC_ALL wins")
	}
	clear()
	t.Setenv("LANGUAGE", "de:fr")
	if Detect() != FR {
		t.Fatal("LANGUAGE list")
	}
	clear()
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "fr_FR.UTF-8")
	if Detect() != FR {
		t.Fatal("C locale is skipped")
	}
}

func TestSetAndPlural(t *testing.T) {
	defer Set(EN)
	if Set("de") || Lang() != EN {
		t.Fatal("unsupported language must be rejected")
	}
	if !Set("FR_ca") || Lang() != FR {
		t.Fatal("fr")
	}
	if N("search.idle", 0) != "0 carte · titre, workflow · / pour chercher" || N("search.idle", 2)[:8] != "2 cartes" {
		t.Fatal(N("search.idle", 0))
	}
	Set("en")
	if N("head.active", 0) != "0 active workflows" || N("head.active", 1) != "1 active workflow" {
		t.Fatal("en plural")
	}
}
