package packages

import (
	"fmt"
	"io"
	"time"

	"github.com/anchore/syft/internal"
	"github.com/anchore/syft/internal/version"

	"github.com/anchore/syft/syft/source"

	"github.com/anchore/syft/internal/log"

	"github.com/anchore/syft/syft/pkg"
	spdxLicense "github.com/mitchellh/go-spdx"
	"github.com/spdx/tools-golang/spdx"
	"github.com/spdx/tools-golang/tvsaver"
)

// SPDXPresenter is a SPDX presentation object for the syft results (see https://github.com/spdx/spdx-spec)
type SPDXPresenter struct {
	catalog     *pkg.Catalog
	srcMetadata source.Metadata
}

// NewJSONPresenter creates a new JSON presenter object for the given cataloging results.
func NewSPDXPresenter(catalog *pkg.Catalog, srcMetadata source.Metadata) *SPDXPresenter {
	return &SPDXPresenter{
		catalog:     catalog,
		srcMetadata: srcMetadata,
	}
}

// Present the catalog results to the given writer.
func (pres *SPDXPresenter) Present(output io.Writer) error {
	doc := spdx.Document2_2{
		CreationInfo: &spdx.CreationInfo2_2{
			// 2.1: SPDX Version; should be in the format "SPDX-2.2"
			// Cardinality: mandatory, one
			SPDXVersion: "SPDX-2.2",

			// 2.2: Data License; should be "CC0-1.0"
			// Cardinality: mandatory, one
			DataLicense: "CC0-1.0",

			// 2.3: SPDX Identifier; should be "DOCUMENT" to represent
			//      mandatory identifier of SPDXRef-DOCUMENT
			// Cardinality: mandatory, one
			SPDXIdentifier: spdx.ElementID("DOCUMENT"),

			// 2.4: Document Name
			// Cardinality: mandatory, one
			DocumentName: pres.srcMetadata.ImageMetadata.UserInput,

			// 2.5: Document Namespace
			// Cardinality: mandatory, one
			// Purpose: Provide an SPDX document specific namespace as a unique absolute Uniform Resource
			// Identifier (URI) as specified in RFC-3986, with the exception of the ‘#’ delimiter. The SPDX
			// Document URI cannot contain a URI "part" (e.g. the "#" character), since the ‘#’ is used in SPDX
			// element URIs (packages, files, snippets, etc) to separate the document namespace from the
			// element’s SPDX identifier. Additionally, a scheme (e.g. “https:”) is required.

			// The URI must be unique for the SPDX document including the specific version of the SPDX document.
			// If the SPDX document is updated, thereby creating a new version, a new URI for the updated
			// document must be used. There can only be one URI for an SPDX document and only one SPDX document
			// for a given URI.

			// Note that the URI does not have to be accessible. It is only intended to provide a unique ID.
			// In many cases, the URI will point to a web accessible document, but this should not be assumed
			// to be the case.

			// TODO: rethink this
			DocumentNamespace: fmt.Sprintf("https://anchore.com/syft/image/%s", pres.srcMetadata.ImageMetadata.UserInput),

			// 2.6: External Document References
			// Cardinality: optional, one or many
			ExternalDocumentReferences: nil,

			// 2.7: License List Version
			// Cardinality: optional, one
			LicenseListVersion: "",

			// 2.8: Creators: may have multiple keys for Person, Organization
			//      and/or Tool
			// Cardinality: mandatory, one or many
			CreatorPersons:       nil,
			CreatorOrganizations: []string{"Anchore, Inc"},
			CreatorTools:         []string{internal.ApplicationName + "-" + version.FromBuild().Version},

			// 2.9: Created: data format YYYY-MM-DDThh:mm:ssZ
			// Cardinality: mandatory, one
			Created: time.Now().Format(time.RFC3339),

			// 2.10: Creator Comment
			// Cardinality: optional, one
			CreatorComment: "",

			// 2.11: Document Comment
			// Cardinality: optional, one
			DocumentComment: "",
		},
		Packages: pres.packages(),
		// TODO: consider adding the following fields
		//UnpackagedFiles: nil,
		//OtherLicenses:   nil,
		//Relationships:   nil,
		//Annotations:     nil,
	}

	return tvsaver.Save2_2(&doc, output)
}

// packages populates all Package Information from the package Catalog (see https://spdx.github.io/spdx-spec/3-package-information/)
func (pres *SPDXPresenter) packages() map[spdx.ElementID]*spdx.Package2_2 {
	results := make(map[spdx.ElementID]*spdx.Package2_2)

	for p := range pres.catalog.Enumerate() {
		// TODO: name should be guaranteed to be unique, but semantically useful (and stable)
		id := fmt.Sprintf("Package-%+v-%s", p.Type, p.Name)

		// source: https://spdx.github.io/spdx-spec/3-package-information/#313-concluded-license
		// The options to populate this field are limited to:
		// A valid SPDX License Expression as defined in Appendix IV;
		// NONE, if the SPDX file creator concludes there is no license available for this package; or
		// NOASSERTION if:
		//   (i) the SPDX file creator has attempted to but cannot reach a reasonable objective determination;
		//   (ii) the SPDX file creator has made no attempt to determine this field; or
		//   (iii) the SPDX file creator has intentionally provided no information (no meaning should be implied by doing so).
		license := "NONE"
		if len(p.Licenses) > 0 {
			// note: we are not supporting complex expressions at this time, only individual licenses
			licenseInfo, err := spdxLicense.License(p.Licenses[0])
			if err != nil {
				log.Warnf("unable to parse SPDX license for package=%+v : %+v", p, err)
				license = "NOASSERTION"
			} else {
				license = licenseInfo.ID
			}
		}

		filesAnalyzed, files := pres.packageFiles(p)

		results[spdx.ElementID(id)] = &spdx.Package2_2{

			// NOT PART OF SPEC
			// flag: does this "package" contain files that were in fact "unpackaged",
			// e.g. included directly in the Document without being in a Package?
			IsUnpackaged: false,

			// 3.1: Package Name
			// Cardinality: mandatory, one
			PackageName: p.Name,

			// 3.2: Package SPDX Identifier: "SPDXRef-[idstring]"
			// Cardinality: mandatory, one
			PackageSPDXIdentifier: spdx.ElementID(id),

			// 3.3: Package Version
			// Cardinality: optional, one
			PackageVersion: p.Version,

			// 3.4: Package File Name
			// Cardinality: optional, one
			PackageFileName: "",

			// 3.5: Package Supplier: may have single result for either Person or Organization,
			//                        or NOASSERTION
			// Cardinality: optional, one
			PackageSupplierPerson:       "",
			PackageSupplierOrganization: "",
			PackageSupplierNOASSERTION:  false,

			// 3.6: Package Originator: may have single result for either Person or Organization,
			//                          or NOASSERTION
			// Cardinality: optional, one
			PackageOriginatorPerson:       "",
			PackageOriginatorOrganization: "",
			PackageOriginatorNOASSERTION:  false,

			// 3.7: Package Download Location
			// Cardinality: mandatory, one
			// NONE if there is no download location whatsoever.
			// NOASSERTION if:
			//   (i) the SPDX file creator has attempted to but cannot reach a reasonable objective determination;
			//   (ii) the SPDX file creator has made no attempt to determine this field; or
			//   (iii) the SPDX file creator has intentionally provided no information (no meaning should be implied by doing so).
			PackageDownloadLocation: "NOASSERTION",

			// 3.8: FilesAnalyzed
			// Cardinality: optional, one; default value is "true" if omitted

			// Purpose: Indicates whether the file content of this package has been available for or subjected to
			// analysis when creating the SPDX document. If false, indicates packages that represent metadata or
			// URI references to a project, product, artifact, distribution or a component. If false, the package
			// must not contain any files.

			// Intent: A package can refer to a project, product, artifact, distribution or a component that is
			// external to the SPDX document.
			FilesAnalyzed: filesAnalyzed,
			// NOT PART OF SPEC: did FilesAnalyzed tag appear?
			IsFilesAnalyzedTagPresent: true,

			// 3.9: Package Verification Code
			// Cardinality: mandatory, one if filesAnalyzed is true / omitted;
			//              zero (must be omitted) if filesAnalyzed is false
			PackageVerificationCode: "",
			// Spec also allows specifying a single file to exclude from the
			// verification code algorithm; intended to enable exclusion of
			// the SPDX document file itself.
			PackageVerificationCodeExcludedFile: "",

			// 3.10: Package Checksum: may have keys for SHA1, SHA256 and/or MD5
			// Cardinality: optional, one or many

			// 3.10.1 Purpose: Provide an independently reproducible mechanism that permits unique identification of
			// a specific package that correlates to the data in this SPDX file. This identifier enables a recipient
			// to determine if any file in the original package has been changed. If the SPDX file is to be included
			// in a package, this value should not be calculated. The SHA-1 algorithm will be used to provide the
			// checksum by default.

			// note: based on the purpose above no discovered checksums should be provided, but instead, only
			// tool-derived checksums.
			PackageChecksumSHA1:   "",
			PackageChecksumSHA256: "",
			PackageChecksumMD5:    "",

			// 3.11: Package Home Page
			// Cardinality: optional, one
			PackageHomePage: "",

			// 3.12: Source Information
			// Cardinality: optional, one
			PackageSourceInfo: "",

			// 3.13: Concluded License: SPDX License Expression, "NONE" or "NOASSERTION"
			// Cardinality: mandatory, one
			// Purpose: Contain the license the SPDX file creator has concluded as governing the
			// package or alternative values, if the governing license cannot be determined.
			PackageLicenseConcluded: "NOASSERTION",

			// 3.14: All Licenses Info from Files: SPDX License Expression, "NONE" or "NOASSERTION"
			// Cardinality: mandatory, one or many if filesAnalyzed is true / omitted;
			//              zero (must be omitted) if filesAnalyzed is false
			PackageLicenseInfoFromFiles: nil,

			// 3.15: Declared License: SPDX License Expression, "NONE" or "NOASSERTION"
			// Cardinality: mandatory, one
			// Purpose: List the licenses that have been declared by the authors of the package.
			// Any license information that does not originate from the package authors, e.g. license
			// information from a third party repository, should not be included in this field.
			PackageLicenseDeclared: license,

			// 3.16: Comments on License
			// Cardinality: optional, one
			PackageLicenseComments: "",

			// 3.17: Copyright Text: copyright notice(s) text, "NONE" or "NOASSERTION"
			// Cardinality: mandatory, one
			// Purpose: Identify the copyright holders of the package, as well as any dates present. This will be a free form text field extracted from package information files. The options to populate this field are limited to:
			//
			// Any text related to a copyright notice, even if not complete;
			// NONE if the package contains no copyright information whatsoever; or
			// NOASSERTION, if
			//   (i) the SPDX document creator has made no attempt to determine this field; or
			//   (ii) the SPDX document creator has intentionally provided no information (no meaning should be implied by doing so).
			//
			PackageCopyrightText: "NOASSERTION",

			// 3.18: Package Summary Description
			// Cardinality: optional, one
			PackageSummary: "",

			// 3.19: Package Detailed Description
			// Cardinality: optional, one
			PackageDescription: "",

			// 3.20: Package Comment
			// Cardinality: optional, one
			PackageComment: "",

			// 3.21: Package External Reference
			// Cardinality: optional, one or many
			PackageExternalReferences: nil,

			// 3.22: Package External Reference Comment
			// Cardinality: conditional (optional, one) for each External Reference
			// contained within PackageExternalReference2_1 struct, if present

			// 3.23: Package Attribution Text
			// Cardinality: optional, one or many
			PackageAttributionTexts: nil,

			// Files contained in this Package
			Files: files,
		}
	}
	return results
}

func (pres *SPDXPresenter) packageFiles(p *pkg.Package) (bool, map[spdx.ElementID]*spdx.File2_2) {
	filesAnalyzed := false
	files := make(map[spdx.ElementID]*spdx.File2_2)
	if owner, ok := p.Metadata.(pkg.FileOwner); ok {
		filesAnalyzed = true
		for _, f := range owner.OwnedFiles() {
			// TODO: should we include layer information in the element id?
			id := spdx.ElementID(f)
			files[id] = &spdx.File2_2{

				// 4.1: File Name
				// Cardinality: mandatory, one
				FileName: f,

				// 4.2: File SPDX Identifier: "SPDXRef-[idstring]"
				// Cardinality: mandatory, one
				FileSPDXIdentifier: id,

				// 4.3: File Type
				// Cardinality: optional, multiple
				FileType: nil,

				// 4.4: File Checksum: may have keys for SHA1, SHA256 and/or MD5
				// Cardinality: mandatory, one SHA1, others may be optionally provided
				// TODO: we don't have the resolvers at this point, but we could make that available?
				FileChecksumSHA1:   "",
				FileChecksumSHA256: "",
				FileChecksumMD5:    "",

				// 4.5: Concluded License: SPDX License Expression, "NONE" or "NOASSERTION"
				// Cardinality: mandatory, one
				LicenseConcluded: "NOASSERTION",

				// 4.6: License Information in File: SPDX License Expression, "NONE" or "NOASSERTION"
				// Cardinality: mandatory, one or many
				// TODO: could use a license classifier here
				LicenseInfoInFile: []string{"NOASSERTION"},

				// 4.7: Comments on License
				// Cardinality: optional, one
				LicenseComments: "",

				// 4.8: Copyright Text: copyright notice(s) text, "NONE" or "NOASSERTION"
				// Cardinality: mandatory, one
				FileCopyrightText: "NOASSERTION",

				// DEPRECATED in version 2.1 of spec
				// 4.9-4.11: Artifact of Project variables (defined below)
				// Cardinality: optional, one or many
				ArtifactOfProjects: nil,

				// 4.12: File Comment
				// Cardinality: optional, one
				FileComment: "",

				// 4.13: File Notice
				// Cardinality: optional, one
				FileNotice: "",

				// 4.14: File Contributor
				// Cardinality: optional, one or many
				FileContributor: nil,

				// 4.15: File Attribution Text
				// Cardinality: optional, one or many
				FileAttributionTexts: nil,

				// DEPRECATED in version 2.0 of spec
				// 4.16: File Dependencies
				// Cardinality: optional, one or many
				FileDependencies: nil,

				// Snippets contained in this File
				// Note that Snippets could be defined in a different Document! However,
				// the only ones that _THIS_ document can contain are this ones that are
				// defined here -- so this should just be an ElementID.
				Snippets: nil,
			}
		}
	}

	return filesAnalyzed, files
}
