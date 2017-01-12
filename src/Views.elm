module Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import String
import Tuple


-- Internal Imports

import Types exposing (..)
import Utils.Date exposing (..)
import Utils.Views exposing (..)
import Silences.Views


view : Model -> Html Msg
view model =
    case model.route of
        AlertGroupsRoute ->
            genericListView alertGroupsView model.alertGroups

        NewSilenceRoute ->
            Silences.Views.silenceFormView "New" model.silence

        EditSilenceRoute id ->
            Silences.Views.silenceFormView "Edit" model.silence

        SilencesRoute ->
            genericListView silenceListView model.silences

        SilenceRoute name ->
            Silences.Views.silenceView model.silence

        _ ->
            notFoundView model


todoView : a -> Html Msg
todoView model =
    div []
        [ h1 [] [ text "todo" ]
        ]


alertGroupsView : AlertGroup -> Html Msg
alertGroupsView alertGroup =
    li [ class "pa3 pa4-ns bb b--black-10" ]
        [ div [ class "mb3" ] (List.map alertHeader <| List.sort alertGroup.labels)
        , div [] (List.map blockView alertGroup.blocks)
        ]


blockView : Block -> Html Msg
blockView block =
    -- Block level
    div []
        (List.map alertView block.alerts)


alertView : Alert -> Html Msg
alertView alert =
    let
        id =
            case alert.silenceId of
                Just id ->
                    id

                Nothing ->
                    0

        b =
            if alert.silenced then
                buttonLink "fa-deaf" ("#/silences/" ++ toString id) "dark-blue" (Noop [])
            else
                buttonLink "fa-exclamation-triangle" "#/silences/new" "dark-red" <|
                    (SilenceFromAlert (List.map (\( k, v ) -> Matcher k v False) alert.labels))
    in
        div [ class "f6 mb3" ]
            [ div [ class "mb1" ]
                [ b
                , buttonLink "fa-bar-chart" alert.generatorUrl "black" (Noop [])
                , p [ class "dib mr2" ] [ text <| Utils.Date.dateFormat alert.startsAt ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map labelButton <| List.filter (\( k, v ) -> k /= "alertname") alert.labels)
            ]


buttonLink : String -> String -> String -> msg -> Html msg
buttonLink icon link color msg =
    a [ class <| "f6 link br1 ba mr1 ph3 pv2 mb2 dib " ++ color, href link, onClick msg ]
        [ i [ class <| "fa fa-3 " ++ icon ] []
        ]


alertHeader : ( String, String ) -> Html msg
alertHeader ( key, value ) =
    if key == "alertname" then
        b [ class "db f4 mr2 dark-red dib" ] [ text value ]
    else
        listButton "ph1 pv1" ( key, value )


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


silenceListView : Silence -> Html Msg
silenceListView silence =
    let
        -- TODO: Check with fabxc if the alert being in the first position can
        -- be relied upon.
        alertName =
            case List.head silence.matchers of
                Just m ->
                    m.value

                Nothing ->
                    ""
    in
        li
            [ class "pa3 pa4-ns bb b--black-10" ]
            [ a
                [ class "db link blue"
                , href ("#/silences/" ++ (toString silence.id))
                ]
                [ b [ class "db f4 mb1" ]
                    [ text alertName ]
                ]
            , span [ class "f5 db lh-copy measure" ]
                [ text silence.createdBy ]
            , span [ class "f5 db lh-copy measure" ]
                [ text silence.comment ]
            ]
