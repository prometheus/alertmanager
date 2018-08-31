module Silences.Decoders exposing (create, destroy, list, show)

import Json.Decode as Json exposing (fail, field, succeed)
import Silences.Types exposing (Silence, State(..), Status)
import Utils.Api exposing (andMap, iso8601Time)
import Utils.Types exposing (ApiData(..), Matcher, Time)


show : Json.Decoder Silence
show =
    Json.at [ "data" ] silenceDecoder


list : Json.Decoder (List Silence)
list =
    Json.at [ "data" ] (Json.list silenceDecoder)


create : Json.Decoder String
create =
    Json.at [ "data", "silenceId" ] Json.string


destroy : Json.Decoder String
destroy =
    Json.at [ "status" ] Json.string


silenceDecoder : Json.Decoder Silence
silenceDecoder =
    Json.succeed Silence
        |> andMap (field "id" Json.string)
        |> andMap (field "createdBy" Json.string)
        -- Remove this maybe once the api either disallows empty comments on
        -- creation, or returns an empty string.
        |> andMap
            (Json.maybe (field "comment" Json.string)
                |> Json.andThen (\x -> Json.succeed <| Maybe.withDefault "" x)
            )
        |> andMap (field "startsAt" iso8601Time)
        |> andMap (field "endsAt" iso8601Time)
        |> andMap (field "updatedAt" iso8601Time)
        |> andMap (field "matchers" (Json.list matcherDecoder))
        |> andMap (field "status" statusDecoder)


statusDecoder : Json.Decoder Status
statusDecoder =
    Json.succeed Status
        |> andMap (field "state" Json.string |> Json.andThen stateDecoder)


stateDecoder : String -> Json.Decoder State
stateDecoder state =
    case state of
        "active" ->
            succeed Active

        "pending" ->
            succeed Pending

        "expired" ->
            succeed Expired

        _ ->
            fail <|
                "Silence.status.state must be one of 'active', 'pending' or 'expired' but was'"
                    ++ state
                    ++ "'."


matcherDecoder : Json.Decoder Matcher
matcherDecoder =
    Json.map3 Matcher
        (field "isRegex" Json.bool)
        (field "name" Json.string)
        (field "value" Json.string)
