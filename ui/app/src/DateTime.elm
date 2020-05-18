module DateTime exposing (DateTime, decoder, encoder, toDate, toString)

import Date exposing (Date, fromPosix)
import Iso8601
import Json.Decode as Decode exposing (Decoder)
import Json.Encode as Encode
import Result
import Time exposing (utc)


type alias DateTime =
    Time.Posix


decoder : Decoder DateTime
decoder =
    Decode.string
        |> Decode.andThen decodeIsoString


encoder : DateTime -> Encode.Value
encoder =
    Encode.string << toString


decodeIsoString : String -> Decoder DateTime
decodeIsoString str =
    case Iso8601.toTime str of
        Result.Ok posix ->
            Decode.succeed posix

        Result.Err _ ->
            Decode.fail <| "Invalid date: " ++ str


toString : DateTime -> String
toString =
    Iso8601.fromTime


toDate : Time.Posix -> Date
toDate time =
    fromPosix utc time
