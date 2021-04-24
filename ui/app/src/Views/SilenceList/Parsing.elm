module Views.SilenceList.Parsing exposing (silenceListParser)

import Url.Parser exposing ((<?>), Parser, map, s)
import Url.Parser.Query as Query
import Utils.Filter exposing (Filter)


boolParam : String -> Query.Parser Bool
boolParam name =
    Query.custom name (List.head >> (/=) Nothing)


maybeBoolParam : String -> Query.Parser (Maybe Bool)
maybeBoolParam name =
    Query.custom name
        (List.head >> Maybe.map (String.toLower >> (/=) "false"))


silenceListParser : Parser (Filter -> a) a
silenceListParser =
    s "silences"
        <?> Query.string "filter"
        <?> Query.string "creator"
        <?> Query.string "group"
        <?> boolParam "customGrouping"
        <?> Query.string "receiver"
        <?> maybeBoolParam "silenced"
        <?> maybeBoolParam "inhibited"
        <?> maybeBoolParam "active"
        |> map Filter
