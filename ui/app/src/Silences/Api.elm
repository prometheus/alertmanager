module Silences.Api exposing (..)

import Http
import Silences.Types exposing (Silence)
import Utils.Types exposing (ApiData(..))
import Utils.Filter exposing (Filter)
import Utils.Api
import Silences.Decoders exposing (show, list, create, destroy)
import Silences.Encoders
import Utils.Filter exposing (generateQueryString)


getSilences : String -> Filter -> (ApiData (List Silence) -> msg) -> Cmd msg
getSilences apiUrl filter msg =
    let
        url =
            String.join "/" [ apiUrl, "silences" ++ (generateQueryString filter) ]
    in
        Utils.Api.send (Utils.Api.get url list)
            |> Cmd.map msg


getSilence : String -> String -> (ApiData Silence -> msg) -> Cmd msg
getSilence apiUrl uuid msg =
    let
        url =
            String.join "/" [ apiUrl, "silence", uuid ]
    in
        Utils.Api.send (Utils.Api.get url show)
            |> Cmd.map msg


create : String -> Silence -> Cmd (ApiData String)
create apiUrl silence =
    let
        url =
            String.join "/" [ apiUrl, "silences" ]

        body =
            Http.jsonBody <| Silences.Encoders.silence silence
    in
        -- TODO: This should return the silence, not just the ID, so that we can
        -- redirect to the silence show page.
        Utils.Api.send
            (Utils.Api.post url body Silences.Decoders.create)


destroy : String -> Silence -> (ApiData String -> msg) -> Cmd msg
destroy apiUrl silence msg =
    -- The incorrect route using "silences" receives a 405. The route seems to
    -- be matching on /silences and ignoring the :sid, should be getting a 404.
    let
        url =
            String.join "/" [ apiUrl, "silence", silence.id ]

        responseDecoder =
            -- Silences.Encoders.silence silence
            Silences.Decoders.destroy
    in
        Utils.Api.send (Utils.Api.delete url responseDecoder)
            |> Cmd.map msg
