module Updates exposing (update)

import Browser.Navigation as Navigation
import Task
import Types exposing (Model, Msg(..), Route(..))
import Views.AlertList.Types exposing (AlertListMsg(..))
import Views.AlertList.Updates
import Views.Settings.Updates
import Views.SilenceForm.Types exposing (SilenceFormMsg(..))
import Views.SilenceForm.Updates
import Views.SilenceList.Types exposing (SilenceListMsg(..))
import Views.SilenceList.Updates
import Views.SilenceView.Types as SilenceViewTypes
import Views.SilenceView.Updates
import Views.Status.Types exposing (StatusMsg(..))
import Views.Status.Updates


update : Msg -> Model -> ( Model, Cmd Msg )
update msg ({ basePath, apiUrl } as model) =
    case msg of
        NavigateToAlerts filter ->
            let
                ( alertList, cmd ) =
                    Views.AlertList.Updates.update FetchAlerts model.alertList filter apiUrl basePath
            in
            ( { model | alertList = alertList, route = AlertsRoute filter, filter = filter }, cmd )

        NavigateToSilenceList filter ->
            let
                ( silenceList, cmd ) =
                    Views.SilenceList.Updates.update FetchSilences model.silenceList filter basePath apiUrl
            in
            ( { model | silenceList = silenceList, route = SilenceListRoute filter, filter = filter }
            , Cmd.map MsgForSilenceList cmd
            )

        NavigateToStatus ->
            ( { model | route = StatusRoute }, Task.perform identity (Task.succeed <| MsgForStatus <| InitStatusView apiUrl) )

        NavigateToSilenceView silenceId ->
            let
                ( silenceView, cmd ) =
                    Views.SilenceView.Updates.update (SilenceViewTypes.InitSilenceView silenceId) model.silenceView apiUrl
            in
            ( { model | route = SilenceViewRoute silenceId, silenceView = silenceView }
            , Cmd.map MsgForSilenceView cmd
            )

        NavigateToSilenceFormNew params ->
            ( { model | route = SilenceFormNewRoute params }
            , Task.perform (NewSilenceFromMatchersAndComment model.defaultCreator >> MsgForSilenceForm) (Task.succeed params)
            )

        NavigateToSilenceFormEdit uuid ->
            ( { model | route = SilenceFormEditRoute uuid }, Task.perform identity (Task.succeed <| (FetchSilence uuid |> MsgForSilenceForm)) )

        NavigateToNotFound ->
            ( { model | route = NotFoundRoute }, Cmd.none )

        NavigateToInternalUrl url ->
            ( model, Navigation.pushUrl model.key url )

        NavigateToExternalUrl url ->
            ( model, Navigation.load url )

        RedirectAlerts ->
            ( model, Navigation.pushUrl model.key (basePath ++ "#/alerts") )

        NavigateToSettings ->
            ( { model | route = SettingsRoute }, Cmd.none )

        MsgForStatus subMsg ->
            Views.Status.Updates.update subMsg model

        MsgForAlertList subMsg ->
            let
                ( alertList, cmd ) =
                    Views.AlertList.Updates.update subMsg model.alertList model.filter apiUrl basePath
            in
            ( { model | alertList = alertList }, cmd )

        MsgForSilenceList subMsg ->
            let
                ( silenceList, cmd ) =
                    Views.SilenceList.Updates.update subMsg model.silenceList model.filter basePath apiUrl
            in
            ( { model | silenceList = silenceList }, Cmd.map MsgForSilenceList cmd )

        MsgForSettings subMsg ->
            let
                ( settingsView, cmd ) =
                    Views.Settings.Updates.update subMsg model.settings
            in
            ( { model | settings = settingsView }, cmd )

        MsgForSilenceView subMsg ->
            let
                ( silenceView, cmd ) =
                    Views.SilenceView.Updates.update subMsg model.silenceView apiUrl
            in
            ( { model | silenceView = silenceView }, Cmd.map MsgForSilenceView cmd )

        MsgForSilenceForm subMsg ->
            let
                ( silenceForm, cmd ) =
                    Views.SilenceForm.Updates.update subMsg model.silenceForm basePath apiUrl
            in
            ( { model | silenceForm = silenceForm }, cmd )

        BootstrapCSSLoaded css ->
            ( { model | bootstrapCSS = css }, Cmd.none )

        FontAwesomeCSSLoaded css ->
            ( { model | fontAwesomeCSS = css }, Cmd.none )

        ElmDatepickerCSSLoaded css ->
            ( { model | elmDatepickerCSS = css }, Cmd.none )

        SetDefaultCreator name ->
            ( { model | defaultCreator = name }, Cmd.none )

        SetGroupExpandAll expanded ->
            ( { model | expandAll = expanded }, Cmd.none )
