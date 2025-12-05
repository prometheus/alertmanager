module Views.AlertList.Parsing exposing (alertsParser)

import Url.Parser exposing ((<?>), Parser, map, s)
import Url.Parser.Query as Query
import Utils.Filter exposing (Filter, MatchOperator(..))


boolParam : String -> Query.Parser Bool
boolParam name =
    Query.custom name (List.head >> (/=) Nothing)


maybeBoolParam : String -> Query.Parser (Maybe Bool)
maybeBoolParam name =
    Query.custom name
        (List.head >> Maybe.map (String.toLower >> (/=) "false"))


alertsParser : Parser (Filter -> a) a
alertsParser =
    s "alerts"
        <?> Query.string "filter"
        <?> Query.string "group"
        <?> boolParam "customGrouping"
        <?> Query.string "receiver"
        <?> maybeBoolParam "silenced"
        <?> maybeBoolParam "inhibited"
        <?> maybeBoolParam "muted"
        <?> maybeBoolParam "active"
        |> map Filter
