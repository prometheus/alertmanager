module Views.Silence.Parsing exposing (silenceParser)

import UrlParser exposing (Parser, s, string, (</>))


silenceParser : Parser (String -> a) a
silenceParser =
    s "silences" </> string
