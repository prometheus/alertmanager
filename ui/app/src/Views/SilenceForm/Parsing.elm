module Views.SilenceForm.Parsing exposing (newSilenceFromAlertLabels, silenceFormNewParser, silenceFormEditParser)

import UrlParser exposing (Parser, s, (</>), (<?>), string, stringParam, oneOf, map)
import Utils.Filter exposing (parseFilter, Matcher)
import Http exposing (encodeUri)


newSilenceFromAlertLabels : List ( String, String ) -> String
newSilenceFromAlertLabels labels =
    labels
        |> List.map (\( k, v ) -> Utils.Filter.Matcher k Utils.Filter.Eq v)
        |> Utils.Filter.stringifyFilter
        |> encodeUri
        |> (++) "#/silences/new?filter="


silenceFormNewParser : Parser (List Matcher -> a) a
silenceFormNewParser =
    s "silences"
        </> s "new"
        <?> stringParam "filter"
        |> map (Maybe.andThen parseFilter >> Maybe.withDefault [])


silenceFormEditParser : Parser (String -> a) a
silenceFormEditParser =
    s "silences" </> string </> s "edit"
