module Utils.Api exposing (..)

import Http exposing (Error(..))
import Json.Decode as Json exposing (field)
import Time exposing (Time)
import Utils.Date
import Utils.Types exposing (ApiData(..))


withDefault : a -> ApiData a -> a
withDefault default response =
    case response of
        Success value ->
            value

        _ ->
            default


parseError : String -> Maybe String
parseError =
    Json.decodeString (field "error" Json.string) >> Result.toMaybe


errorToString : Http.Error -> String
errorToString err =
    case err of
        Timeout ->
            "Timeout exceeded"

        NetworkError ->
            "Network error"

        BadStatus resp ->
            parseError resp.body
                |> Maybe.withDefault (toString resp.status.code ++ " " ++ resp.status.message)

        BadPayload err resp ->
            -- OK status, unexpected payload
            "Unexpected response from api: " ++ err

        BadUrl url ->
            "Malformed url: " ++ url


fromResult : Result Http.Error a -> ApiData a
fromResult result =
    case result of
        Err e ->
            Failure (errorToString e)

        Ok x ->
            Success x


send : Http.Request a -> Cmd (ApiData a)
send =
    Http.send fromResult


get : String -> Json.Decoder a -> Http.Request a
get url decoder =
    request "GET" [] url Http.emptyBody decoder


post : String -> Http.Body -> Json.Decoder a -> Http.Request a
post url body decoder =
    request "POST" [] url body decoder


delete : String -> Json.Decoder a -> Http.Request a
delete url decoder =
    request "DELETE" [] url Http.emptyBody decoder


request : String -> List Http.Header -> String -> Http.Body -> Json.Decoder a -> Http.Request a
request method headers url body decoder =
    Http.request
        { method = method
        , headers = headers
        , url = url
        , body = body
        , expect = Http.expectJson decoder
        , timeout = Nothing
        , withCredentials = False
        }


iso8601Time : Json.Decoder Time
iso8601Time =
    Json.andThen
        (\strTime ->
            case Utils.Date.timeFromString strTime of
                Ok time ->
                    Json.succeed time

                Err err ->
                    Json.fail ("Could not decode time " ++ strTime ++ ": " ++ err)
        )
        Json.string


makeApiUrl : String -> String
makeApiUrl externalUrl =
    let
        url =
            if String.endsWith "/" externalUrl then
                String.dropRight 1 externalUrl
            else
                externalUrl
    in
        url ++ "/api/v1"


defaultTimeout : Time.Time
defaultTimeout =
    1000 * Time.millisecond


(|:) : Json.Decoder (a -> b) -> Json.Decoder a -> Json.Decoder b
(|:) =
    -- Taken from elm-community/json-extra
    flip (Json.map2 (|>))
