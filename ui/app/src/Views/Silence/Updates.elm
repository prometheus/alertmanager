module Views.Silence.Updates exposing (update)

import Views.Silence.Types exposing (SilenceMsg(..))
import Types exposing (Model, Msg(MsgForSilence))
import Silences.Api exposing (getSilence)
import Alerts.Api
import Utils.List
import Utils.Types exposing (ApiResponse(..), nullFilter)


update : SilenceMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        FetchSilence id ->
            ( model, getSilence id (SilenceFetched >> MsgForSilence) )

        AlertGroupsPreview alertGroups ->
            case model.silence of
                Success silence ->
                    ( { model
                        | silence =
                            Success
                                { silence | silencedAlertGroups = alertGroups }
                      }
                    , Cmd.none
                    )

                _ ->
                    ( model, Cmd.none )

        SilenceFetched (Success silence) ->
            ( { model
                | silence = Success { silence | silencedAlertGroups = Loading }
              }
            , Alerts.Api.alertGroups
                ({ nullFilter | text = Just (Utils.List.mjoin silence.matchers) })
                |> Cmd.map (AlertGroupsPreview >> MsgForSilence)
            )

        SilenceFetched silence ->
            ( { model | silence = silence }, Cmd.none )

        InitSilenceView silenceId ->
            ( model, getSilence silenceId (SilenceFetched >> MsgForSilence) )
