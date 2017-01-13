module Silences.Api exposing (..)

import Http
import Types exposing (Msg(..))
import Silences.Decoders exposing (..)
import Utils.Api exposing (baseUrl)


getSilences : Cmd Msg
getSilences =
    let
        url =
            String.join "/" [ baseUrl, "silences?limit=1000" ]
    in
        Http.send SilencesFetch (Http.get url list)


getSilence : Int -> Cmd Msg
getSilence id =
    let
        url =
            String.join "/" [ baseUrl, "silence", toString id ]
    in
        Http.send SilenceFetch (Http.get url show)
