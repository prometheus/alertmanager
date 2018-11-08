module Silences.Api exposing (create, destroy, getSilence, getSilences)

import Data.Silence exposing (Silence)
import Data.Silences
import Http
import Silences.Decoders
import Utils.Api
import Utils.Filter exposing (Filter, generateQueryString)
import Utils.Types exposing (ApiData(..))


getSilences : String -> Filter -> (ApiData (List Silence) -> msg) -> Cmd msg
getSilences apiUrl filter msg =
    let
        url =
            String.join "/" [ apiUrl, "silences" ++ generateQueryString filter ]
    in
    Utils.Api.send (Utils.Api.get url Data.Silences.decoder)
        |> Cmd.map msg


getSilence : String -> String -> (ApiData Silence -> msg) -> Cmd msg
getSilence apiUrl uuid msg =
    let
        url =
            String.join "/" [ apiUrl, "silence", uuid ]
    in
    Utils.Api.send (Utils.Api.get url Data.Silence.decoder)
        |> Cmd.map msg


create : String -> Silence -> Cmd (ApiData String)
create apiUrl silence =
    let
        url =
            String.join "/" [ apiUrl, "silences" ]

        body =
            Http.jsonBody <| Data.Silence.encoder silence
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
            -- TODO: Maybe.withDefault is not perfect. Silences should always
            -- have an id, what should we do?
            String.join "/" [ apiUrl, "silence", Maybe.withDefault "" silence.id ]

        responseDecoder =
            -- Silences.Encoders.silence silence
            Silences.Decoders.destroy
    in
    Utils.Api.send (Utils.Api.delete url responseDecoder)
        |> Cmd.map msg
