module Utils.Api exposing (..)

import Json.Decode as Json
import ISO8601


stringtoISO8601 : Json.Decoder ISO8601.Time
stringtoISO8601 =
    Json.string
        |> Json.andThen
            (\val ->
                case ISO8601.fromString val of
                    Err err ->
                        Json.fail err

                    Ok date ->
                        Json.succeed <| date
            )


baseUrl : String
baseUrl =
    "http://alertmanager.int.s-cloud.net/api/v1"



-- "http://localhost:9093/api/v1"
