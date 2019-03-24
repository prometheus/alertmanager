module Views.SilenceView.Parsing exposing (silenceViewParser)

import Url.Parser exposing ((</>), Parser, s, string)


silenceViewParser : Parser (String -> a) a
silenceViewParser =
    s "silences" </> string
