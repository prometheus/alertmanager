module Utils.Api exposing (delete, get, makeApiUrl, map, post, send)

import Http exposing (Error(..))
import Json.Decode as Json exposing (field)
import Utils.Types exposing (ApiData(..))


map : (a -> b) -> ApiData a -> ApiData b
map fn response =
    case response of
        Success value ->
            Success (fn value)

        Initial ->
            Initial

        Loading ->
            Loading

        Failure a ->
            Failure a


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
                |> Maybe.withDefault (String.fromInt resp.status.code ++ " " ++ resp.status.message)

        BadPayload err_ _ ->
            -- OK status, unexpected payload
            "Unexpected response from api: " ++ err_

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


makeApiUrl : String -> String
makeApiUrl externalUrl =
    let
        url =
            if String.endsWith "/" externalUrl then
                String.dropRight 1 externalUrl

            else
                externalUrl
    in
    url ++ "/api/v2"
