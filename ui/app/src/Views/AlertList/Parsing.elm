module Views.AlertList.Parsing exposing (alertsParser)

import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam)
import Utils.Filter exposing (Filter, parseMatcher, MatchOperator(RegexMatch))


boolParam : String -> UrlParser.QueryParser (Maybe Bool -> a) a
boolParam name =
    UrlParser.customParam name
        (Maybe.map (String.toLower >> (/=) "false"))


alertsParser : Parser (Filter -> a) a
alertsParser =
    s "alerts"
        <?> stringParam "filter"
        <?> stringParam "group"
        <?> stringParam "receiver"
        <?> boolParam "silenced"
        <?> boolParam "inhibited"
        |> map Filter
