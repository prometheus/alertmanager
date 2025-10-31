module Views exposing (view)

import Html exposing (Html, div, node, text)
import Html.Attributes exposing (class, href, rel, style)
import Html.Events exposing (on)
import Json.Decode exposing (succeed)
import Types exposing (Model, Msg(..), Route(..))
import Utils.Filter exposing (emptySilenceFormGetParams)
import Utils.Types exposing (ApiData(..))
import Utils.Views
import Views.AlertList.Views as AlertList
import Views.NavBar.Views exposing (navBar)
import Views.NotFound.Views as NotFound
import Views.Settings.Views as SettingsView
import Views.SilenceForm.Views as SilenceForm
import Views.SilenceList.Views as SilenceList
import Views.SilenceView.Views as SilenceView
import Views.Status.Views as Status


view : Model -> Html Msg
view model =
    div []
        [ renderCSS model.libUrl
        , case ( model.bootstrapCSS, model.fontAwesomeCSS, model.elmDatepickerCSS ) of
            ( Success _, Success _, Success _ ) ->
                div []
                    [ navBar model.route
                    , div [ class "container pb-4" ] [ currentView model ]
                    ]

            ( Failure err, _, _ ) ->
                failureView model err

            ( _, Failure err, _ ) ->
                failureView model err

            ( _, _, Failure err ) ->
                failureView model err

            _ ->
                text ""
        ]


failureView : Model -> String -> Html Msg
failureView model err =
    div []
        [ div [ style "padding" "40px", style "color" "red" ] [ text err ]
        , navBar model.route
        , div [ class "container pb-4" ] [ currentView model ]
        ]


renderCSS : String -> Html Msg
renderCSS assetsUrl =
    div []
        [ cssNode (assetsUrl ++ "lib/bootstrap-4.0.0-alpha.6-dist/css/bootstrap.min.css") BootstrapCSSLoaded
        , cssNode (assetsUrl ++ "lib/font-awesome-4.7.0/css/font-awesome.min.css") FontAwesomeCSSLoaded
        , cssNode (assetsUrl ++ "lib/elm-datepicker/css/elm-datepicker.css") ElmDatepickerCSSLoaded
        ]


cssNode : String -> (ApiData String -> Msg) -> Html Msg
cssNode url msg =
    node "link"
        [ href url
        , rel "stylesheet"
        , on "load" (succeed (msg (Success url)))
        , on "error" (succeed (msg (Failure ("Failed to load CSS from: " ++ url))))
        ]
        []


currentView : Model -> Html Msg
currentView model =
    case model.route of
        SettingsRoute ->
            SettingsView.view model.settings |> Html.map MsgForSettings

        StatusRoute ->
            Status.view model.status

        SilenceViewRoute _ ->
            SilenceView.view model.silenceView

        AlertsRoute filter ->
            AlertList.view model.alertList filter

        SilenceListRoute _ ->
            SilenceList.view model.silenceList

        SilenceFormNewRoute getParams ->
            SilenceForm.view Nothing getParams model.defaultCreator model.silenceForm |> Html.map MsgForSilenceForm

        SilenceFormEditRoute silenceId ->
            SilenceForm.view (Just silenceId) emptySilenceFormGetParams "" model.silenceForm |> Html.map MsgForSilenceForm

        TopLevelRoute ->
            Utils.Views.loading

        NotFoundRoute ->
            NotFound.view
