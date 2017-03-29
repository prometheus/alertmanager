module Utils.Api exposing (..)

import Http
import Json.Decode as Json exposing (field)
import Time exposing (Time)
import Utils.Date
import Utils.Types exposing (ApiData, ApiResponse(..))


fromResult : Result e a -> ApiResponse e a
fromResult result =
    case result of
        Err e ->
            Failure e

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
                Just time ->
                    Json.succeed time

                Nothing ->
                    Json.fail ("Could not decode time " ++ strTime)
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
