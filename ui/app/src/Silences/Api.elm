module Silences.Api exposing (..)

import Http
import Silences.Types exposing (Silence)
import Utils.Types exposing (Filter, ApiResponse(..), ApiData)
import Silences.Decoders exposing (..)
import Silences.Encoders
import Utils.Api exposing (baseUrl)
import Utils.Filter exposing (generateQueryString)


getSilences : Filter -> (ApiData (List Silence) -> msg) -> Cmd msg
getSilences filter msg =
    let
        url =
            String.join "/" [ baseUrl, "silences" ++ (generateQueryString filter) ]
    in
        Utils.Api.send (Utils.Api.get url list)
            |> Cmd.map msg


getSilence : String -> (ApiData Silence -> msg) -> Cmd msg
getSilence uuid msg =
    let
        url =
            String.join "/" [ baseUrl, "silence", uuid ]
    in
        Utils.Api.send (Utils.Api.get url show)
            |> Cmd.map msg


create : Silence -> Cmd (ApiData String)
create silence =
    let
        url =
            String.join "/" [ baseUrl, "silences" ]

        body =
            Http.jsonBody <| Silences.Encoders.silence silence
    in
        -- TODO: This should return the silence, not just the ID, so that we can
        -- redirect to the silence show page.
        Utils.Api.send
            (Utils.Api.post url body Silences.Decoders.create)


destroy : Silence -> (ApiData String -> msg) -> Cmd msg
destroy silence msg =
    -- The incorrect route using "silences" receives a 405. The route seems to
    -- be matching on /silences and ignoring the :sid, should be getting a 404.
    let
        url =
            String.join "/" [ baseUrl, "silence", silence.id ]

        responseDecoder =
            -- Silences.Encoders.silence silence
            Silences.Decoders.destroy
    in
        Utils.Api.send (Utils.Api.delete url responseDecoder)
            |> Cmd.map msg
