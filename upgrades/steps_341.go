// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func stepsFor341() []Step {
	return []Step{}
}

func stateStepsFor341() []Step {
	return []Step{
		&upgradeStep{
			description: "fill in empty charmhub charm origin tracks to latest",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().FillInEmptyCharmhubTracks()
			},
		},
	}
}
