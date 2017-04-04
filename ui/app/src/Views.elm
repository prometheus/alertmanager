module Views exposing (..)

import Html exposing (Html, text, div)
import Html.Attributes exposing (class)
import Types exposing (Msg(MsgForSilenceForm), Model, Route(..))
import Utils.Types exposing (ApiResponse(..))
import Utils.Views exposing (error, loading)
import Views.SilenceList.Views as SilenceList
import Views.SilenceForm.Views as SilenceForm
import Views.AlertList.Views as AlertList
import Views.Silence.Views as Silence
import Views.NotFound.Views as NotFound
import Views.Status.Views as Status
import Views.NavBar.Views exposing (appHeader)


view : Model -> Html Msg
view model =
    div []
        [ appHeader links
        , div [ class "container" ]
            [ currentView model ]
        ]


links : List ( String, String )
links =
    [ ( "#/alerts", "Alerts" )
    , ( "#/silences", "Silences" )
    , ( "#/status", "Status" )
    ]


currentView : Model -> Html Msg
currentView model =
    case model.route of
        StatusRoute ->
            Status.view model

        SilenceRoute silenceId ->
            Silence.view model

        AlertsRoute filter ->
            case model.alertGroups of
                Success alertGroups ->
                    AlertList.view alertGroups model.filter (text "")

                Loading ->
                    loading

                Failure msg ->
                    AlertList.view [] model.filter (error msg)

        SilenceListRoute route ->
            SilenceList.view model.silences model.silence model.currentTime model.filter

        SilenceFormNewRoute keep ->
            SilenceForm.view Nothing model.silenceForm |> Html.map MsgForSilenceForm

        SilenceFormEditRoute silenceId ->
            SilenceForm.view (Just silenceId) model.silenceForm |> Html.map MsgForSilenceForm

        TopLevelRoute ->
            Utils.Views.loading

        NotFoundRoute ->
            NotFound.view
