module Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)


-- Internal Imports

import Types exposing (..)
import Silences.Views
import Alerts.Views


view : Model -> Html Msg
view model =
    if model.loading then
        loading
    else
        case model.route of
            AlertsRoute route ->
                Html.map Alerts (Alerts.Views.view route model.alertGroups)

            NewSilenceRoute ->
                Silences.Views.silenceForm "New" model.silence

            EditSilenceRoute id ->
                Silences.Views.silenceForm "Edit" model.silence

            SilencesRoute ->
                -- Add buttons at the top to filter Active/Pending/Expired
                genericListView Silences.Views.silenceList model.silences

            SilenceRoute name ->
                Silences.Views.silence model.silence

            _ ->
                notFoundView model


loading : Html msg
loading =
    div []
        [ i [ class "fa fa-cog fa-spin fa-3x fa-fw" ] []
        , span [ class "sr-only" ] [ text "Loading..." ]
        ]


todoView : a -> Html Msg
todoView model =
    div []
        [ h1 [] [ text "todo" ]
        ]


notFoundView : a -> Html Msg
notFoundView model =
    div []
        [ h1 [] [ text "not found" ]
        ]


genericListView : (a -> Html Msg) -> List a -> Html Msg
genericListView fn list =
    ul
        [ classList
            [ ( "list", True )
            , ( "pa0", True )
            ]
        ]
        (List.map fn list)
