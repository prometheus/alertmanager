package cli

import "gopkg.in/alecthomas/kingpin.v2"

// silenceCmd represents the silence command
func configureSilenceCmd(app *kingpin.Application) {
	silenceCmd := app.Command("silence", "Add, expire or view silences. For more information and additional flags see query help").PreAction(requireAlertManagerURL)
	configureSilenceAddCmd(silenceCmd)
	configureSilenceExpireCmd(silenceCmd)
	configureSilenceImportCmd(silenceCmd)
	configureSilenceQueryCmd(silenceCmd)
	configureSilenceUpdateCmd(silenceCmd)
}
