module Utils.Api exposing (..)

import Json.Decode as Json exposing (field)
import Utils.Types exposing (ApiResponse(..), ApiData, Time, Duration)
import Http
import Time
import Utils.Date


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


duration : String -> String -> Json.Decoder Duration
duration startsAt endsAt =
    Json.map2
        (\t1 t2 ->
            case ( t1.t, t2.t ) of
                ( Just time1, Just time2 ) ->
                    Utils.Date.duration (time2 - time1)

                _ ->
                    Utils.Date.duration 0
        )
        (Json.field startsAt iso8601Time)
        (Json.field endsAt iso8601Time)


iso8601Time : Json.Decoder Time
iso8601Time =
    Json.map Utils.Date.timeFromString Json.string


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
