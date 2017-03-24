module Views.SilenceList.Parsing exposing (silenceListParser)

import UrlParser exposing ((<?>), Parser, s, stringParam)


silenceListParser : Parser (Maybe String -> a) a
silenceListParser =
    s "silences" <?> stringParam "filter"
