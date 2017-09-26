module Views.Status.Views exposing (view)

import Html exposing (..)
import Html.Attributes exposing (class, style)
import Status.Types exposing (StatusResponse, VersionInfo, MeshStatus, MeshPeer, Route, Parent)
import Types exposing (Msg(MsgForStatus))
import Utils.Types exposing (ApiData(Failure, Success, Loading, Initial), Matcher)
import Views.Status.Types exposing (StatusModel)
import Utils.Views
import Graph.Tree as Tree exposing (Tree)


view : StatusModel -> Html Msg
view statusInfo =
    case statusInfo of
        Success info ->
            viewStatusInfo info

        Initial ->
            Utils.Views.loading

        Loading ->
            Utils.Views.loading

        Failure msg ->
            Utils.Views.error msg


viewStatusInfo : StatusResponse -> Html Msg
viewStatusInfo status =
    div []
        [ h1 [] [ text "Status" ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Uptime:" ]
            , div [ class "col-sm-10" ] [ text status.uptime ]
            ]
        , viewMeshStatus status.meshStatus
        , viewVersionInformation status.versionInfo
        , viewConfig status.config
        , routingTree status.route
        ]


makeTree : Route -> Maybe Status.Types.Parent -> Tree Route
makeTree node parent =
    let
        newParent =
            Just (Status.Types.Parent node)

        leaves =
            case node.routes of
                Just (Status.Types.Routes routes) ->
                    List.map (flip makeTree newParent) routes

                Nothing ->
                    []
    in
        Tree.inner { node | parent = parent } leaves


type alias Node =
    { matchers : List Matcher
    , x : Int
    , y : Int
    }


routingTree : Route -> Html Msg
routingTree route =
    let
        tree =
            makeTree route Nothing

        x =
            Tree.postOrder
                (\route routes acc ->
                    let
                        new =
                            case route.parent of
                                Just (Status.Types.Parent parent) ->
                                    let
                                        x =
                                            case parent.routes of
                                                Just (Status.Types.Routes routes) ->
                                                    indexOf route routes

                                                Nothing ->
                                                    0

                                        depth =
                                            1 + (findDepth parent)
                                    in
                                        { matchers = route.matchers, x = x, y = depth }

                                Nothing ->
                                    { matchers = route.matchers, x = 0, y = 0 }
                    in
                        new :: acc
                )
                []
                tree
    in
        div [] []


indexOf : Route -> List Route -> Int
indexOf route routes =
    case routes of
        x :: xs ->
            if route == x then
                0
            else
                1 + (indexOf route xs)

        [] ->
            0


findDepth : Route -> Int
findDepth route =
    case route.parent of
        Just (Status.Types.Parent route) ->
            1 + (findDepth route)

        Nothing ->
            0


viewConfig : String -> Html Msg
viewConfig config =
    div []
        [ h2 [] [ text "Config" ]
        , pre [ class "p-4", style [ ( "background", "#f7f7f9" ), ( "font-family", "monospace" ) ] ]
            [ code []
                [ text config
                ]
            ]
        ]


viewMeshStatus : Maybe MeshStatus -> Html Msg
viewMeshStatus meshStatus =
    case meshStatus of
        Just meshStatus ->
            span []
                [ h2 [] [ text "Mesh Status" ]
                , div [ class "form-group row" ]
                    [ b [ class "col-sm-2" ] [ text "Name:" ]
                    , div [ class "col-sm-10" ] [ text meshStatus.name ]
                    ]
                , div [ class "form-group row" ]
                    [ b [ class "col-sm-2" ] [ text "Nick Name:" ]
                    , div [ class "col-sm-10" ] [ text meshStatus.nickName ]
                    ]
                , div [ class "form-group row" ]
                    [ b [ class "col-sm-2" ] [ text "Peers:" ]
                    , ul [ class "col-sm-10" ] <|
                        List.map viewMeshPeer meshStatus.peers
                    ]
                ]

        Nothing ->
            span []
                [ h2 [] [ text "Mesh Status" ]
                , div [ class "form-group row" ]
                    [ div [ class "col-sm-10" ] [ text "Mesh not configured" ]
                    ]
                ]


viewMeshPeer : MeshPeer -> Html Msg
viewMeshPeer peer =
    li []
        [ div [ class "" ]
            [ b [ class "" ] [ text "Name: " ]
            , text peer.name
            ]
        , div [ class "" ]
            [ b [ class "" ] [ text "Nick Name: " ]
            , text peer.nickName
            ]
        , div [ class "" ]
            [ b [ class "" ] [ text "UID: " ]
            , text <| toString peer.uid
            ]
        ]


viewVersionInformation : VersionInfo -> Html Msg
viewVersionInformation versionInfo =
    span []
        [ h2 [] [ text "Version Information" ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Branch:" ], div [ class "col-sm-10" ] [ text versionInfo.branch ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "BuildDate:" ], div [ class "col-sm-10" ] [ text versionInfo.buildDate ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "BuildUser:" ], div [ class "col-sm-10" ] [ text versionInfo.buildUser ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "GoVersion:" ], div [ class "col-sm-10" ] [ text versionInfo.goVersion ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Revision:" ], div [ class "col-sm-10" ] [ text versionInfo.revision ] ]
        , div [ class "form-group row" ]
            [ b [ class "col-sm-2" ] [ text "Version:" ], div [ class "col-sm-10" ] [ text versionInfo.version ] ]
        ]
