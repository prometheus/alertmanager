module Silences.Api exposing (..)

import Http
import Types
import Silences.Types exposing (Silence, SilencesMsg(..), Msg(..))
import Utils.Types exposing (Filter, ApiResponse(..))
import Silences.Decoders exposing (..)
import Silences.Encoders
import Utils.Api exposing (baseUrl)
import Utils.Filter exposing (generateQueryString)


getSilences : Filter -> Cmd Types.Msg
getSilences filter =
    let
        url =
            String.join "/" [ baseUrl, "silences" ++ (generateQueryString filter) ]
    in
        Utils.Api.send (Utils.Api.get url list)
            |> Cmd.map Types.SilencesFetch


getSilence : String -> Cmd Types.Msg
getSilence uuid =
    let
        url =
            String.join "/" [ baseUrl, "silence", uuid ]
    in
        Utils.Api.send (Utils.Api.get url show)
            |> Cmd.map Types.SilenceFetch


create : Silence -> Cmd Msg
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
            |> Cmd.map (SilenceCreate >> ForSelf)


destroy : Silence -> Cmd Msg
destroy silence =
    -- The incorrect route using "silences" receives a 405. The route seems to
    -- be matching on /silences and ignoring the :sid, should be getting a 404.
    let
        url =
            String.join "/" [ baseUrl, "silence", silence.id ]

        responseDecoder =
            -- Silences.Encoders.silence silence
            Silences.Decoders.destroy
    in
        Utils.Api.send
            (Utils.Api.delete url responseDecoder)
            |> Cmd.map (SilenceDestroy >> ForSelf)
