module Silences.Api exposing (..)

import Http
import Types exposing (Silence, Msg(..))
import Silences.Decoders exposing (..)
import Silences.Encoders
import Utils.Api exposing (baseUrl)


getSilences : Cmd Msg
getSilences =
    let
        -- Can remove limit=1000 when we are on version >= 0.5.x
        url =
            String.join "/" [ baseUrl, "silences?limit=1000" ]
    in
        -- TODO: Talk with fabxc about adding some sort of filter for e.g.
        -- entering values into a search bar or clicking on labels
        Http.send SilencesFetch (Http.get url list)


getSilence : Int -> Cmd Msg
getSilence id =
    let
        url =
            String.join "/" [ baseUrl, "silence", toString id ]
    in
        Http.send SilenceFetch (Http.get url show)


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
        Http.send SilenceCreate
            (Http.post url body Silences.Decoders.create)


destroy : Silence -> Cmd Msg
destroy silence =
    -- The incorrect route using "silences" receives a 405. The route seems to
    -- be matching on /silences and ignoring the :sid, should be getting a 404.
    let
        url =
            String.join "/" [ baseUrl, "silence", toString silence.id ]

        body =
            Http.jsonBody <| Silences.Encoders.silence silence
    in
        -- TODO: This should return the silence, not just the ID, so that we can
        -- redirect to the silence show page.
        Http.send SilenceDestroy
            (Http.request
                { method = "DELETE"
                , headers = []
                , url = url
                , body =
                    Http.emptyBody
                    -- Body being sent
                , expect =
                    (Http.expectJson Silences.Decoders.create)
                    -- Response expected
                , timeout =
                    Nothing
                    -- Just (200 * Time.millisecond)
                , withCredentials = False
                }
            )
