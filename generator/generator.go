package generators

import (
	"github.com/Seann-Moser/mserve"
)

type Language string

const (
	LanguageGo = Language("go")
)

type GeneratorData struct {
	ProjectName string
	RootDir     string
	Title       string
	Version     string
	Description string
	Host        string
}

func NewGenData() GeneratorData {
	gd := GeneratorData{
		ProjectName: "",
		RootDir:     "",
		Title:       "",
		Version:     "",
		Description: "",
		Host:        "",
	}
	gd.RootDir, _ = GetRootDir()
	gd.ProjectName, _ = GetProjectName()
	return gd
}

type Generator interface {
	Generate(data GeneratorData, endpoints ...mserve.Endpoint) error
}
