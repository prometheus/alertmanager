module Views.SilenceList.Parsing exposing (silenceListParser)

import UrlParser exposing ((<?>), Parser, s, stringParam, map)
import Utils.Types exposing (Filter)


silenceListParser : Parser (Filter -> a) a
silenceListParser =
    map
        (\t ->
            Filter t Nothing Nothing
        )
        (s "silences" <?> stringParam "filter")
