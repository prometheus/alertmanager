module Silences.Api exposing (create, destroy, getSilence, getSilences)

import Data.GettableSilence exposing (GettableSilence)
import Data.PostableSilence exposing (PostableSilence)
import Http
import Json.Decode
import Silences.Decoders
import Utils.Api
import Utils.Filter exposing (Filter, generateQueryString)
import Utils.Types exposing (ApiData(..))


getSilences : String -> Filter -> (ApiData (List GettableSilence) -> msg) -> Cmd msg
getSilences apiUrl filter msg =
    let
        url =
            String.join "/" [ apiUrl, "silences" ++ generateQueryString filter ]
    in
    Utils.Api.send (Utils.Api.get url (Json.Decode.list Data.GettableSilence.decoder))
        |> Cmd.map msg


getSilence : String -> String -> (ApiData GettableSilence -> msg) -> Cmd msg
getSilence apiUrl uuid msg =
    let
        url =
            String.join "/" [ apiUrl, "silence", uuid ]
    in
    Utils.Api.send (Utils.Api.get url Data.GettableSilence.decoder)
        |> Cmd.map msg


create : String -> PostableSilence -> Cmd (ApiData String)
create apiUrl silence =
    let
        url =
            String.join "/" [ apiUrl, "silences" ]

        body =
            Http.jsonBody <| Data.PostableSilence.encoder silence
    in
    -- TODO: This should return the silence, not just the ID, so that we can
    -- redirect to the silence show page.
    Utils.Api.send
        (Utils.Api.post url body Silences.Decoders.create)


destroy : String -> GettableSilence -> (ApiData String -> msg) -> Cmd msg
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
