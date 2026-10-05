package publication

import (
	"strings"
	"testing"
)

func TestLocationLabelsRequirePortableBasenames(t *testing.T) {
	for _, field := range []string{"directory", "repository"} {
		for _, name := range []string{"", "fixture-project", "fixture project", "fixture.项目"} {
			t.Run(field+" valid "+name, func(t *testing.T) {
				fact := locationLabelFact(field, name)
				if err := ValidateFact(fact); err != nil {
					t.Fatal(err)
				}
			})
		}
		for _, name := range []string{"/synthetic/private/project", "C:/synthetic/private/project", `C:\synthetic\private\project`, `\\synthetic-server\share\project`, "relative/project", `relative\project`, "~/synthetic/project"} {
			t.Run(field+" invalid "+name, func(t *testing.T) {
				err := ValidateFact(locationLabelFact(field, name))
				if err == nil {
					t.Fatal("path label accepted")
				}
				if strings.Contains(err.Error(), name) {
					t.Fatal("rejected path echoed in validation error")
				}
			})
		}
	}
}

func locationLabelFact(field, name string) Fact {
	fact := fixtureFact()
	fact.Location = &Location{DirectoryKey: "fixture-directory-key", DirectoryName: "fixture-directory", RepositoryKey: "fixture-repository-key", RepositoryName: "fixture-repository"}
	if field == "directory" {
		fact.Location.DirectoryName = name
	} else {
		fact.Location.RepositoryName = name
	}
	SetIDs(&fact)
	return fact
}
