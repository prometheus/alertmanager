module Views exposing (..)

import Html exposing (Html, node, text, div)
import Html.Attributes exposing (class, rel, href, src, style)
import Html.Events exposing (on)
import Json.Decode exposing (succeed)
import Types exposing (Msg(MsgForSilenceForm, MsgForSilenceView, BootstrapCSSLoaded, FontAwesomeCSSLoaded), Model, Route(..))
import Utils.Views exposing (error, loading)
import Utils.Types exposing (ApiData(Failure, Success))
import Views.SilenceList.Views as SilenceList
import Views.SilenceForm.Views as SilenceForm
import Views.AlertList.Views as AlertList
import Views.SilenceView.Views as SilenceView
import Views.NotFound.Views as NotFound
import Views.Status.Views as Status
import Views.NavBar.Views exposing (navBar)


view : Model -> Html Msg
view model =
    div []
        [ renderCSS model.libUrl
        , case ( model.bootstrapCSS, model.fontAwesomeCSS ) of
            ( Success _, Success _ ) ->
                div []
                    [ navBar model.route
                    , div [ class "container pb-4" ] [ currentView model ]
                    ]

            ( Failure err, _ ) ->
                failureView model err

            ( _, Failure err ) ->
                failureView model err

            _ ->
                text ""
        ]


failureView : Model -> String -> Html Msg
failureView model err =
    div []
        [ div [ style [ ( "padding", "40px" ), ( "color", "red" ) ] ] [ text err ]
        , navBar model.route
        , div [ class "container pb-4" ] [ currentView model ]
        ]


renderCSS : String -> Html Msg
renderCSS assetsUrl =
    div []
        [ cssNode (assetsUrl ++ "lib/bootstrap-4.0.0-alpha.6-dist/css/bootstrap.min.css") BootstrapCSSLoaded
        , cssNode (assetsUrl ++ "lib/font-awesome-4.7.0/css/font-awesome.min.css") FontAwesomeCSSLoaded
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
        StatusRoute ->
            Status.view model.status

        SilenceViewRoute silenceId ->
            SilenceView.view model.silenceView

        AlertsRoute filter ->
            AlertList.view model.alertList filter

        SilenceListRoute _ ->
            SilenceList.view model.silenceList

        SilenceFormNewRoute matchers ->
            SilenceForm.view Nothing matchers model.defaultCreator model.silenceForm |> Html.map MsgForSilenceForm

        SilenceFormEditRoute silenceId ->
            SilenceForm.view (Just silenceId) [] "" model.silenceForm |> Html.map MsgForSilenceForm

        TopLevelRoute ->
            Utils.Views.loading

        NotFoundRoute ->
            NotFound.view
