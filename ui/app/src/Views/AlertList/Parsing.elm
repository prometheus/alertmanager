module Views.AlertList.Parsing exposing (alertsParser)

import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam)
import Utils.Filter exposing (Filter, parseMatcher, MatchOperator(Eq, RegexMatch))


boolParam : String -> UrlParser.QueryParser (Maybe Bool -> a) a
boolParam name =
    UrlParser.customParam name
        (\x ->
            case x of
                Nothing ->
                    Nothing

                Just value ->
                    if (String.toLower value) == "false" then
                        Just False
                    else
                        Just True
        )


alertsParser : Parser (Filter -> a) a
alertsParser =
    map
        (\filter receiver silenced ->
            let
                parsed =
                    Maybe.map
                        (\r ->
                            { key = "receiver", op = RegexMatch, value = "^(?:" ++ r ++ ")$" }
                        )
                        receiver
            in
                Filter filter parsed silenced
        )
        (s "alerts" <?> stringParam "filter" <?> stringParam "receiver" <?> boolParam "silenced")
