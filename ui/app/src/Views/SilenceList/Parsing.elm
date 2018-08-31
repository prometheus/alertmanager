module Views.SilenceList.Parsing exposing (silenceListParser)

import UrlParser exposing ((<?>), Parser, map, s, stringParam)
import Utils.Filter exposing (Filter)


silenceListParser : Parser (Filter -> a) a
silenceListParser =
    map
        (\t ->
            Filter t Nothing Nothing Nothing Nothing
        )
        (s "silences" <?> stringParam "filter")
