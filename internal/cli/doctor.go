package cli

import (
	"github.com/Lazy-256/spotify-helper/internal/config"
	"github.com/Lazy-256/spotify-helper/internal/doctor"
	"github.com/Lazy-256/spotify-helper/internal/output"
)

func init() {
	register(&command{
		name:    "doctor",
		usage:   "",
		summary: "check configuration, token and rate-limit state offline",
		run:     runDoctor,
	})
}

func runDoctor(env *Env, args []string) error {
	if _, _, err := parseCommon("doctor", "", args, 0, 0, flagSet{}); err != nil {
		return err
	}
	p, err := env.paths()
	if err != nil {
		return err // cannot even locate the directories
	}
	cfg, cfgErr := config.Load(p, env.Getenv)
	tok, tokErr := env.store(p).Read()
	rep := doctor.Build(doctor.Inputs{
		Paths:              p,
		Config:             cfg,
		ConfigErr:          cfgErr,
		Token:              tok,
		TokenErr:           tokErr,
		RateLimitRemaining: env.gate(p).Remaining(),
		NowUnix:            env.Now().Unix(),
	})
	return output.WriteJSON(env.Stdout, rep)
}
