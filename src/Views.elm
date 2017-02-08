module Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)


-- Internal Imports

import Types exposing (..)
import Translators exposing (alertTranslator)
import Silences.Views
import Alerts.Views


view : Model -> Html Msg
view model =
    case model.route of
        AlertsRoute route ->
            Html.map alertTranslator (Alerts.Views.view route model.alertGroups)

        NewSilenceRoute ->
            case model.silence of
                Success silence ->
                    Silences.Views.silenceForm "New" silence

                Loading ->
                    loading

                _ ->
                    notFoundView model

        EditSilenceRoute id ->
            case model.silence of
                Success silence ->
                    Silences.Views.silenceForm "Edit" silence

                Loading ->
                    loading

                _ ->
                    notFoundView model

        SilencesRoute ->
            -- Add buttons at the top to filter Active/Pending/Expired
            case model.silences of
                Success silences ->
                    apiDataList Silences.Views.silenceList silences

                Loading ->
                    loading

                _ ->
                    notFoundView model

        SilenceRoute name ->
            case model.silence of
                Success silence ->
                    Silences.Views.silence silence

                Loading ->
                    loading

                _ ->
                    notFoundView model

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


apiDataList : (a -> Html Msg) -> List a -> Html Msg
apiDataList fn list =
    ul
        [ classList
            [ ( "list", True )
            , ( "pa0", True )
            ]
        ]
        (List.map fn list)
