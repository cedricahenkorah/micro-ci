package config

import (
	"log"

	"github.com/cedricahenkorah/micro-ci/backend/internal/repository"
)

type Application struct {
	ErrorLog *log.Logger
	InfoLog  *log.Logger
	Models   repository.Models
	Config   Config
}
