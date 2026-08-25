package pass

import pb "github.com/roman-16/proton-cli/internal/service/pass/proto"

// An identity's fields, declared once.
//
// Pass stores about thirty of them, grouped the way its own editor groups them:
// who you are, where you live, how to reach you, where you work. Repeating that
// list in the create path, the update path, the read path, the flag registration
// and the record would be five copies that drift apart, and the CLI surfaced
// thirteen of them for exactly that reason.
//
// So the list lives here, once, and everything else walks it.

// IdentityField is one field of an identity: what a person calls it, and where
// it lives on the stored item.
//
// At is a pointer accessor rather than a getter and a setter, so reading and
// writing a field cannot end up pointing at two different places.
type IdentityField struct {
	// Flag is the command-line name, without dashes.
	Flag string
	// Label is what a record calls it.
	Label string
	// Group is the section Pass's own editor puts it in, so the help reads the
	// way the app looks.
	Group string
	At    func(*pb.ItemIdentity) *string
}

// Get and Set read and write the field on one identity.
func (f IdentityField) Get(i *pb.ItemIdentity) string    { return *f.At(i) }
func (f IdentityField) Set(i *pb.ItemIdentity, v string) { *f.At(i) = v }

// IdentityFields is every field an identity carries, in the order Pass shows
// them.
var IdentityFields = []IdentityField{
	// Who they are.
	{"full-name", "Full Name", "personal", func(i *pb.ItemIdentity) *string { return &i.FullName }},
	{"first-name", "First Name", "personal", func(i *pb.ItemIdentity) *string { return &i.FirstName }},
	{"middle-name", "Middle Name", "personal", func(i *pb.ItemIdentity) *string { return &i.MiddleName }},
	{"last-name", "Last Name", "personal", func(i *pb.ItemIdentity) *string { return &i.LastName }},
	{"birthdate", "Birthdate", "personal", func(i *pb.ItemIdentity) *string { return &i.Birthdate }},
	{"gender", "Gender", "personal", func(i *pb.ItemIdentity) *string { return &i.Gender }},

	// Where to reach them.
	{"email", "Email", "personal", func(i *pb.ItemIdentity) *string { return &i.Email }},
	{"phone", "Phone", "personal", func(i *pb.ItemIdentity) *string { return &i.PhoneNumber }},
	{"second-phone", "Second Phone", "contact", func(i *pb.ItemIdentity) *string { return &i.SecondPhoneNumber }},

	// Where they live.
	{"organization", "Organization", "address", func(i *pb.ItemIdentity) *string { return &i.Organization }},
	{"address", "Address", "address", func(i *pb.ItemIdentity) *string { return &i.StreetAddress }},
	{"floor", "Floor", "address", func(i *pb.ItemIdentity) *string { return &i.Floor }},
	{"city", "City", "address", func(i *pb.ItemIdentity) *string { return &i.City }},
	{"county", "County", "address", func(i *pb.ItemIdentity) *string { return &i.County }},
	{"state", "State", "address", func(i *pb.ItemIdentity) *string { return &i.StateOrProvince }},
	{"postal-code", "Postal Code", "address", func(i *pb.ItemIdentity) *string { return &i.ZipOrPostalCode }},
	{"country", "Country", "address", func(i *pb.ItemIdentity) *string { return &i.CountryOrRegion }},

	// Numbers a government gave them.
	{"social-security-number", "Social Security Number", "contact", func(i *pb.ItemIdentity) *string { return &i.SocialSecurityNumber }},
	{"passport-number", "Passport Number", "contact", func(i *pb.ItemIdentity) *string { return &i.PassportNumber }},
	{"license-number", "License Number", "contact", func(i *pb.ItemIdentity) *string { return &i.LicenseNumber }},

	// Where they are online.
	{"website", "Website", "contact", func(i *pb.ItemIdentity) *string { return &i.Website }},
	{"x-handle", "X Handle", "contact", func(i *pb.ItemIdentity) *string { return &i.XHandle }},
	{"linkedin", "LinkedIn", "contact", func(i *pb.ItemIdentity) *string { return &i.Linkedin }},
	{"reddit", "Reddit", "contact", func(i *pb.ItemIdentity) *string { return &i.Reddit }},
	{"facebook", "Facebook", "contact", func(i *pb.ItemIdentity) *string { return &i.Facebook }},
	{"instagram", "Instagram", "contact", func(i *pb.ItemIdentity) *string { return &i.Instagram }},
	{"yahoo", "Yahoo", "contact", func(i *pb.ItemIdentity) *string { return &i.Yahoo }},

	// Where they work.
	{"company", "Company", "work", func(i *pb.ItemIdentity) *string { return &i.Company }},
	{"job-title", "Job Title", "work", func(i *pb.ItemIdentity) *string { return &i.JobTitle }},
	{"work-email", "Work Email", "work", func(i *pb.ItemIdentity) *string { return &i.WorkEmail }},
	{"work-phone", "Work Phone", "work", func(i *pb.ItemIdentity) *string { return &i.WorkPhoneNumber }},
	{"personal-website", "Personal Website", "work", func(i *pb.ItemIdentity) *string { return &i.PersonalWebsite }},
}

// buildIdentity fills a new identity from the values a command line carried.
func buildIdentity(values map[string]string) *pb.ItemIdentity {
	idn := &pb.ItemIdentity{}
	for _, f := range IdentityFields {
		if v := values[f.Flag]; v != "" {
			f.Set(idn, v)
		}
	}
	return idn
}

// patchIdentity lays new values over an existing identity, leaving alone what
// the command line did not mention.
func patchIdentity(idn *pb.ItemIdentity, values map[string]string) {
	for _, f := range IdentityFields {
		if v := values[f.Flag]; v != "" {
			f.Set(idn, v)
		}
	}
}

// readIdentity reports what an identity holds, keyed by the flag that sets it.
func readIdentity(idn *pb.ItemIdentity) map[string]string {
	out := make(map[string]string, len(IdentityFields))
	for _, f := range IdentityFields {
		if v := f.Get(idn); v != "" {
			out[f.Flag] = v
		}
	}
	return out
}
