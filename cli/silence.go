package cli

import "gopkg.in/alecthomas/kingpin.v2"

// silenceCmd represents the silence command
func configureSilenceCmd(app *kingpin.Application, longHelpText map[string]string) {
	silenceCmd := app.Command("silence", "Add, expire or view silences. For more information and additional flags see query help").PreAction(requireAlertManagerURL)
	configureSilenceAddCmd(silenceCmd, longHelpText)
	configureSilenceExpireCmd(silenceCmd, longHelpText)
	configureSilenceImportCmd(silenceCmd, longHelpText)
	configureSilenceQueryCmd(silenceCmd, longHelpText)
	configureSilenceUpdateCmd(silenceCmd, longHelpText)
}
