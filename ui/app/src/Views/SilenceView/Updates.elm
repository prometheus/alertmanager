module Views.SilenceView.Updates exposing (update)

import Alerts.Api
import Navigation exposing (newUrl)
import Silences.Api exposing (getSilence)
import Utils.Filter exposing (nullFilter)
import Utils.List
import Utils.Types exposing (ApiData(..))
import Views.SilenceView.Types exposing (Model, SilenceViewMsg(..))


update : SilenceViewMsg -> Model -> String -> ( Model, Cmd SilenceViewMsg )
update msg model apiUrl =
    case msg of
        FetchSilence id ->
            ( model, getSilence apiUrl id SilenceFetched )

        AlertGroupsPreview alerts ->
            ( { model | alerts = alerts }
            , Cmd.none
            )

        SilenceFetched (Success silence) ->
            ( { model
                | silence = Success silence
                , alerts = Loading
              }
            , Alerts.Api.fetchAlerts
                apiUrl
                { nullFilter | text = Just (Utils.List.mjoin silence.matchers), showSilenced = Just True }
                |> Cmd.map AlertGroupsPreview
            )

        ConfirmDestroySilence silence refresh ->
            ( { model | showConfirmationDialog = True }
            , Cmd.none
            )

        SilenceFetched silence ->
            ( { model | silence = silence, alerts = Initial }, Cmd.none )

        InitSilenceView silenceId ->
            ( { model | showConfirmationDialog = False }, getSilence apiUrl silenceId SilenceFetched )

        Reload silenceId ->
            ( { model | showConfirmationDialog = False }, newUrl ("#/silences/" ++ silenceId) )
