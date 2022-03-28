module Views.SilenceForm.Parsing exposing (newSilenceFromAlertLabels, newSilenceFromMatchers, newSilenceFromMatchersAndComment, silenceFormEditParser, silenceFormNewParser)

import Data.Matcher
import Dict exposing (Dict)
import Url exposing (percentEncode)
import Url.Parser exposing ((</>), (<?>), Parser, s, string)
import Url.Parser.Query as Query
import Utils.Filter exposing (SilenceFormGetParams, parseFilter)


newSilenceFromAlertLabels : Dict String String -> String
newSilenceFromAlertLabels labels =
    labels
        |> Dict.toList
        |> List.map (\( k, v ) -> Utils.Filter.Matcher k Utils.Filter.Eq v)
        |> encodeMatchers


parseGetParams : Maybe String -> Maybe String -> SilenceFormGetParams
parseGetParams filter comment =
    { matchers = filter |> Maybe.andThen parseFilter >> Maybe.withDefault []
    , comment = comment |> Maybe.withDefault ""
    }


silenceFormNewParser : Parser (SilenceFormGetParams -> a) a
silenceFormNewParser =
    s "silences"
        </> s "new"
        <?> Query.map2 parseGetParams (Query.string "filter") (Query.string "comment")


silenceFormEditParser : Parser (String -> a) a
silenceFormEditParser =
    s "silences" </> string </> s "edit"


newSilenceFromMatchers : List Data.Matcher.Matcher -> String
newSilenceFromMatchers matchers =
    matchers
        |> List.map
            (\{ name, value, isRegex, isEqual } ->
                let
                    isEqualValue =
                        case isEqual of
                            Nothing ->
                                True

                            Just justIsEqual ->
                                justIsEqual

                    op =
                        if not isRegex && isEqualValue then
                            Utils.Filter.Eq

                        else if not isRegex && not isEqualValue then
                            Utils.Filter.NotEq

                        else if isRegex && isEqualValue then
                            Utils.Filter.RegexMatch

                        else
                            Utils.Filter.NotRegexMatch
                in
                Utils.Filter.Matcher name op value
            )
        |> encodeMatchers


newSilenceFromMatchersAndComment : List Data.Matcher.Matcher -> String -> String
newSilenceFromMatchersAndComment matchers comment =
    newSilenceFromMatchers matchers ++ "&comment=" ++ (comment |> percentEncode)


encodeMatchers : List Utils.Filter.Matcher -> String
encodeMatchers matchers =
    matchers
        |> Utils.Filter.stringifyFilter
        |> percentEncode
        |> (++) "#/silences/new?filter="
