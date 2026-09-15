module Views.SilenceList.Parsing exposing (silenceListParser)

import Url.Parser exposing ((<?>), Parser, map, s)
import Url.Parser.Query as Query
import Utils.Filter exposing (Filter)


silenceListParser : Parser (Filter -> a) a
silenceListParser =
    map
        (\t ->
            Filter t Nothing False Nothing Nothing Nothing Nothing Nothing
        )
        (s "silences" <?> Query.string "filter")
