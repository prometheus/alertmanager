module Parsing exposing (routeParser, urlParser)

import Regex
import Types exposing (Route(..))
import Url exposing (Url)
import Url.Parser exposing ((</>), (<?>), Parser, int, map, oneOf, parse, s, string, top)
import Views.AlertList.Parsing exposing (alertsParser)
import Views.SilenceForm.Parsing exposing (silenceFormEditParser, silenceFormNewParser)
import Views.SilenceList.Parsing exposing (silenceListParser)
import Views.SilenceView.Parsing exposing (silenceViewParser)
import Views.Status.Parsing exposing (statusParser)


urlParser : Url -> Route
urlParser url =
    let
        -- Parse a query string occurring after the hash if it exists, and use
        -- it for routing.
        hashAndQuery =
            url.fragment
                |> Maybe.map
                    (Regex.splitAtMost 1 (Regex.fromString "\\?" |> Maybe.withDefault Regex.never))
                |> Maybe.withDefault []

        ( path, query ) =
            case hashAndQuery of
                [] ->
                    ( "/", Nothing )

                h :: [] ->
                    ( h, Nothing )

                h :: rest ->
                    ( h, Just (String.join "" rest) )
    in
    case parse routeParser { url | query = query, fragment = Nothing, path = path } of
        Just route ->
            route

        Nothing ->
            NotFoundRoute


routeParser : Parser (Route -> a) a
routeParser =
    oneOf
        [ map SilenceListRoute silenceListParser
        , map StatusRoute statusParser
        , map SilenceFormNewRoute silenceFormNewParser
        , map SilenceViewRoute silenceViewParser
        , map SilenceFormEditRoute silenceFormEditParser
        , map AlertsRoute alertsParser
        , map TopLevelRoute top
        ]
