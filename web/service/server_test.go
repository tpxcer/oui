package service

import "testing"

func TestJoinGeoAddressKeepsFullAddress(t *testing.T) {
	got := joinGeoAddress("中国", "台湾省", "彰化县", "埔盐乡")
	if got != "中国台湾省彰化县埔盐乡" {
		t.Fatalf("joinGeoAddress() = %q", got)
	}
}

func TestJoinGeoAddressDoesNotDuplicateDetail(t *testing.T) {
	got := joinGeoAddress("中国", "台湾省", "彰化县", "埔盐乡", "埔盐乡")
	if got != "中国台湾省彰化县埔盐乡" {
		t.Fatalf("joinGeoAddress() with duplicate detail = %q", got)
	}
}

func TestJoinGeoAddressUsesFullDetailWhenProvided(t *testing.T) {
	got := joinGeoAddress("中国", "台湾省", "彰化县", "埔盐乡", "中国台湾省彰化县埔盐乡")
	if got != "中国台湾省彰化县埔盐乡" {
		t.Fatalf("joinGeoAddress() with full detail = %q", got)
	}
}
