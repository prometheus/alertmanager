module Alerts.Api exposing (..)

import Alerts.Types exposing (Alert, RouteOpts, Block, AlertGroup)
import Json.Decode as Json exposing (..)
import Utils.Api exposing (baseUrl, iso8601Time)
import Utils.Types exposing (ApiData)
import Utils.Filter exposing (Filter, generateQueryString)


fetchAlerts : Filter -> Cmd (ApiData (List Alert))
fetchAlerts filter =
    let
        url =
            String.join "/" [ baseUrl, "alerts" ++ (generateQueryString filter) ]
    in
        Utils.Api.send (Utils.Api.get url alertsDecoder)



-- Decoders
-- Once the API returns the newly created silence, this can go away and we
-- re-use the silence decoder.


alertsDecoder : Json.Decoder (List Alert)
alertsDecoder =
    Json.at [ "data" ] (Json.list alertDecoder)


unwrapWithDefault : a -> Maybe a -> Json.Decoder a
unwrapWithDefault default val =
    case val of
        Just a ->
            Json.succeed a

        Nothing ->
            Json.succeed default


alertDecoder : Json.Decoder Alert
alertDecoder =
    Json.map6 Alert
        (Json.maybe (field "annotations" (Json.keyValuePairs Json.string)) |> andThen (unwrapWithDefault []))
        (field "labels" (Json.keyValuePairs Json.string))
        (Json.maybe (field "silenced" Json.string))
        (decodeSilenced)
        (field "startsAt" iso8601Time)
        (field "generatorURL" Json.string)


decodeSilenced : Decoder Bool
decodeSilenced =
    Json.maybe (field "silenced" Json.string)
        |> andThen
            (\val ->
                case val of
                    Just _ ->
                        Json.succeed True

                    Nothing ->
                        Json.succeed False
            )
