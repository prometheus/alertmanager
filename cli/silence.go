package cli

// silenceCmd represents the silence command
var (
	silenceCmd   = app.Command("silence", "Add, expire or view silences. For more information and additional flags see query help")
	silenceQuiet = silenceCmd.Flag("quiet", "Only show silence ids").Short('q').Bool()
)
