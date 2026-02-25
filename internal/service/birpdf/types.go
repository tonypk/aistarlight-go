package birpdf

// CompanyData holds company/taxpayer metadata for BIR form rendering.
type CompanyData struct {
	Name           string
	TIN            string
	RDOCode        string
	Address        string
	LineOfBusiness string
}
