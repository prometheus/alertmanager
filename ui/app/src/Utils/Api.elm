module Utils.Api exposing (..)

import Http exposing (Error(..))
import Json.Decode as Json exposing (field)
import Time exposing (Time)
import Utils.Date
import Utils.Types exposing (ApiData, ApiResponse(..))


withDefault : a -> ApiResponse e a -> a
withDefault default response =
    case response of
        Success value ->
            value

        _ ->
            default


errorToString : Http.Error -> String
errorToString err =
    case err of
        Timeout ->
            "timeout exceeded"

        NetworkError ->
            "network error"

        BadStatus resp ->
            resp.status.message ++ " " ++ resp.body

        BadPayload err resp ->
            -- OK status, unexpected payload
            "unexpected response from api" ++ err

        BadUrl url ->
            "malformed url: " ++ url


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
        , timeout = Just defaultTimeout
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


baseUrl : String
baseUrl =
    "/api/v1"


defaultTimeout : Time.Time
defaultTimeout =
    1000 * Time.millisecond


(|:) : Json.Decoder (a -> b) -> Json.Decoder a -> Json.Decoder b
(|:) =
    -- Taken from elm-community/json-extra
    flip (Json.map2 (|>))



-- "http://localhost:9093/api/v1"
