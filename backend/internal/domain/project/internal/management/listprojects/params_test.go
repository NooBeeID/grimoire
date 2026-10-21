package listprojects

import (
	"net/url"
	"testing"
)

func TestParseQuery_Shape(t *testing.T) {
	_, fe := parseQuery(url.Values{"limit": {"abc"}, "offset": {"1.5"}})
	if fe["limit"] != "harus berupa angka bulat" || fe["offset"] != "harus berupa angka bulat" {
		t.Fatal(fe)
	}
	in, fe := parseQuery(url.Values{"limit": {""}, "x": {"y"}})
	if len(fe) != 0 || in.Limit != nil || in.Offset != nil {
		t.Fatalf("%+v %v", in, fe)
	}
	in, fe = parseQuery(url.Values{"limit": {"20"}, "q": {" a "}})
	if len(fe) != 0 || in.Limit == nil || *in.Limit != 20 || in.Query != " a " {
		t.Fatalf("%+v %v", in, fe)
	}
}
