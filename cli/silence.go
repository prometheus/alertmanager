package cli

import "github.com/alecthomas/kingpin"

// silenceCmd represents the silence command
func configureSilenceCmd(app *kingpin.Application, longHelpText map[string]string) {
	silenceCmd := app.Command("silence", "Add, expire or view silences. For more information and additional flags see query help")
	configureSilenceAddCmd(silenceCmd, longHelpText)
	configureSilenceExpireCmd(silenceCmd, longHelpText)
	configureSilenceImportCmd(silenceCmd, longHelpText)
	configureSilenceQueryCmd(silenceCmd, longHelpText)
	configureSilenceUpdateCmd(silenceCmd, longHelpText)
}
