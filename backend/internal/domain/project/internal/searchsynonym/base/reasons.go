package base

// Reason adalah kode alasan stabil yang dikirim sebagai details.reason. "" = valid.
type Reason string

const (
	ReasonTooFewTerms     Reason = "too_few_terms"
	ReasonTooManyTerms    Reason = "too_many_terms"
	ReasonTermEmpty       Reason = "term_empty"
	ReasonTermTooLong     Reason = "term_too_long"
	ReasonTermInvalidChar Reason = "term_invalid_char"
	ReasonDuplicateTerm   Reason = "duplicate_term"
	ReasonInvalidGroupID  Reason = "invalid_group_id"
	ReasonInvalidProject  Reason = "invalid_project_id" // bus
	ReasonTermInUse       Reason = "term_in_use"        // 409
	ReasonGroupLimit      Reason = "group_limit"        // 409
)

var reasonMessages = map[Reason]string{
	ReasonTooFewTerms:     "kelompok sinonim minimal berisi 2 term",
	ReasonTooManyTerms:    "kelompok sinonim maksimal berisi 10 term",
	ReasonTermEmpty:       "term tidak boleh kosong",
	ReasonTermTooLong:     "setiap term maksimal 64 karakter",
	ReasonTermInvalidChar: "term hanya boleh berisi huruf, angka, spasi, - dan _",
	ReasonDuplicateTerm:   "term yang sama muncul lebih dari sekali dalam satu kelompok",
	ReasonInvalidGroupID:  "ID kelompok tidak valid",
	ReasonInvalidProject:  "ProjectID tidak valid",
	ReasonTermInUse:       "term sudah dipakai kelompok sinonim lain di project ini",
	ReasonGroupLimit:      "project sudah mencapai batas jumlah kelompok sinonim",
}

// Message mengembalikan pesan Indonesia yang memandu (dipakai HTTP dan bus).
func (r Reason) Message() string {
	if m, ok := reasonMessages[r]; ok {
		return m
	}
	return string(r)
}
